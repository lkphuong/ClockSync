//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/kardianos/service"
	"github.com/robfig/cron/v3"
	"golang.org/x/sys/windows"
)

const (
	timeDriftThreshold = 10 * time.Minute

	systemTimePrivilegeName = "SeSystemtimePrivilege"

	timeAPIURL        = "https://timeapi.io/api/time/current/zone?timeZone=Asia/Ho_Chi_Minh"
	timeAPITimeLayout = "2006-01-02T15:04:05"
)

var procSetSystemTime = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetSystemTime")

type program struct {
	job *cron.Cron
}

type timeAPIResponse struct {
	DateTime string `json:"dateTime"`
}

func (p *program) getTimeFromTimeAPI() (time.Time, error) {
	resp, err := http.Get(timeAPIURL)
	if err != nil {
		return time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return time.Time{}, fmt.Errorf("timeapi.io trả về mã lỗi %d", resp.StatusCode)
	}

	var payload timeAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return time.Time{}, fmt.Errorf("không parse được phản hồi từ timeapi.io: %w", err)
	}

	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		return time.Time{}, err
	}

	return time.ParseInLocation(timeAPITimeLayout, payload.DateTime, loc)
}

func enableSystemTimePrivilege() error {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token); err != nil {
		return fmt.Errorf("không mở được process token: %w", err)
	}
	defer token.Close()

	privilegeName, err := windows.UTF16PtrFromString(systemTimePrivilegeName)
	if err != nil {
		return err
	}

	var luid windows.LUID
	if err := windows.LookupPrivilegeValue(nil, privilegeName, &luid); err != nil {
		return fmt.Errorf("không tra được LUID cho %s: %w", systemTimePrivilegeName, err)
	}

	privileges := windows.Tokenprivileges{
		PrivilegeCount: 1,
		Privileges: [1]windows.LUIDAndAttributes{
			{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED},
		},
	}

	if err := windows.AdjustTokenPrivileges(token, false, &privileges, 0, nil, nil); err != nil {
		return fmt.Errorf("không bật được %s: %w", systemTimePrivilegeName, err)
	}
	return nil
}

func (p *program) setWindowsTime(t time.Time) error {
	if err := enableSystemTimePrivilege(); err != nil {
		return err
	}

	utcTime := t.UTC()
	systemTime := windows.Systemtime{
		Year:   uint16(utcTime.Year()),
		Month:  uint16(utcTime.Month()),
		Day:    uint16(utcTime.Day()),
		Hour:   uint16(utcTime.Hour()),
		Minute: uint16(utcTime.Minute()),
		Second: uint16(utcTime.Second()),
	}

	ret, _, callErr := procSetSystemTime.Call(uintptr(unsafe.Pointer(&systemTime)))
	if ret == 0 {
		return fmt.Errorf("SetSystemTime thất bại: %w", callErr)
	}
	return nil
}

func (p *program) isTimeDrifted(localTime, referenceTime time.Time, threshold time.Duration) bool {
	drift := localTime.Sub(referenceTime)
	if drift < 0 {
		drift = -drift
	}
	return drift > threshold
}

func (p *program) syncSystemTimeIfDrifted() error {
	referenceTime, err := p.getTimeFromTimeAPI()
	if err != nil {
		return fmt.Errorf("không lấy được giờ từ timeapi.io: %w", err)
	}

	localTime := time.Now()

	if !p.isTimeDrifted(localTime, referenceTime, timeDriftThreshold) {
		return nil
	}

	return p.setWindowsTime(referenceTime)
}

func (p *program) Start(s service.Service) error {
	p.job = cron.New()
	p.job.AddFunc("@every 1", func() {
		if err := p.syncSystemTimeIfDrifted(); err != nil {
			fmt.Println("Error:", err)
		}
	})
	p.job.Start()
	return nil
}

func (p *program) Stop(s service.Service) error {
	if p.job != nil {
		p.job.Stop()
	}
	return nil
}

func isRunningElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

func relaunchElevated() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("không xác định được đường dẫn thực thi: %w", err)
	}

	verbPtr, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	exePtr, err := syscall.UTF16PtrFromString(exePath)
	if err != nil {
		return err
	}
	cwdPtr, err := syscall.UTF16PtrFromString(filepath.Dir(exePath))
	if err != nil {
		return err
	}
	argPtr, err := syscall.UTF16PtrFromString(strings.Join(os.Args[1:], " "))
	if err != nil {
		return err
	}

	return windows.ShellExecute(0, verbPtr, exePtr, argPtr, cwdPtr, windows.SW_NORMAL)
}

func main() {
	s, _ := service.New(&program{}, &service.Config{
		Name:        "TimeKeeper",
		DisplayName: "Time Keeper",
		Description: "Automatically syncs the system time with timeapi.io.",
	})

	if service.Interactive() && !isRunningElevated() {
		if err := relaunchElevated(); err != nil {
			log.Fatalf("Cần quyền Administrator để chạy chương trình: %v", err)
		}
		return
	}

	if len(os.Args) > 1 {
		if err := service.Control(s, os.Args[1]); err != nil {
			log.Fatal(err)
		}
		return
	}

	if service.Interactive() {
		log.Println("Install windows service")
		if err := service.Control(s, "install"); err != nil {
			log.Fatal(err)
		}
		log.Println("Start windows service")
		if err := service.Control(s, "start"); err != nil {
			log.Fatal(err)
		}
		log.Println("Service installed and started successfully.")
		time.Sleep(5 * time.Second)
		return
	}

	s.Run()
}

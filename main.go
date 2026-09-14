//go:build windows

package main

import (
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
)

var procSetSystemTime = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetSystemTime")

type program struct {
	job *cron.Cron
}

func (p *program) getTimeFromGoogleHeader() (time.Time, error) {
	resp, err := http.Head("https://google.com")
	if err != nil {
		return time.Time{}, err
	}
	defer resp.Body.Close()

	dateStr := resp.Header.Get("Date")

	gmtTime, err := time.Parse(time.RFC1123, dateStr)
	if err != nil {
		return time.Time{}, err
	}

	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	return gmtTime.In(loc), nil
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
	googleTime, err := p.getTimeFromGoogleHeader()
	if err != nil {
		return fmt.Errorf("không lấy được giờ từ Google: %w", err)
	}

	localTime := time.Now()

	if !p.isTimeDrifted(localTime, googleTime, timeDriftThreshold) {
		return nil
	}

	return p.setWindowsTime(googleTime)
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
		Description: "Automatically syncs the system time with Google's time server.",
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

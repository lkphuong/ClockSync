package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/kardianos/service"

	_ "time/tzdata"
)

type program struct{}

const (
	timeAPIURL         = "https://timeapi.io/api/time/current/zone?timeZone=Asia/Ho_Chi_Minh"
	timeAPITimeLayout  = "2006-01-02T15:04:05"
	systemLocationName = "Asia/Ho_Chi_Minh"

	// Định dạng theo Short Date mặc định của Windows tại Việt Nam (dd/MM/yyyy).
	// Nếu máy đích cấu hình Regional Settings khác, cần đổi lại cho khớp.
	systemDateLayout  = "02/01/2006"
	systemClockLayout = "15:04:05"

	syncInterval = 10 * time.Second
)

type timeAPIResponse struct {
	DateTime string `json:"dateTime"`
}

// getTimeFromTimeAPI lấy thời gian chuẩn theo múi giờ Asia/Ho_Chi_Minh từ timeapi.io.
func (p *program) getTimeFromTimeAPI() (time.Time, error) {
	resp, err := http.Get(timeAPIURL)
	if err != nil {
		return time.Time{}, fmt.Errorf("không gọi được timeapi.io: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return time.Time{}, fmt.Errorf("timeapi.io trả về mã lỗi %d", resp.StatusCode)
	}

	var payload timeAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return time.Time{}, fmt.Errorf("không parse được phản hồi từ timeapi.io: %w", err)
	}

	loc, err := time.LoadLocation(systemLocationName)
	if err != nil {
		return time.Time{}, fmt.Errorf("không tải được múi giờ %s: %w", systemLocationName, err)
	}

	return time.ParseInLocation(timeAPITimeLayout, payload.DateTime, loc)
}

// setSystemDate đặt ngày hệ thống Windows bằng lệnh cmd nội trú "date".
func (p *program) setSystemDate(t time.Time) error {
	dateValue := t.Format(systemDateLayout)
	cmd := exec.Command("cmd", "/C", fmt.Sprintf("date %s", dateValue))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("không đặt được ngày hệ thống (%s): %w", dateValue, err)
	}
	return nil
}

// setSystemClock đặt giờ hệ thống Windows bằng lệnh cmd nội trú "time".
func (p *program) setSystemClock(t time.Time) error {
	timeValue := t.Format(systemClockLayout)
	cmd := exec.Command("cmd", "/C", fmt.Sprintf("time %s", timeValue))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("không đặt được giờ hệ thống (%s): %w", timeValue, err)
	}
	return nil
}

// updateSystemTime đồng bộ ngày giờ hệ thống theo thời gian chuẩn lấy từ timeapi.io.
func (p *program) updateSystemTime() error {
	referenceTime, err := p.getTimeFromTimeAPI()
	if err != nil {
		return err
	}

	if err := p.setSystemDate(referenceTime); err != nil {
		return err
	}

	if err := p.setSystemClock(referenceTime); err != nil {
		return err
	}

	fmt.Println("-> Cập nhật thành công! Hãy kiểm tra đồng hồ máy tính.")
	return nil
}

func (p *program) Start(s service.Service) error {
	go p.run()
	return nil
}

func (p *program) run() {
	if err := p.updateSystemTime(); err != nil {
		fmt.Println("Lỗi:", err)
	}

	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()

	for range ticker.C {
		if err := p.updateSystemTime(); err != nil {
			fmt.Println("Lỗi:", err)
		}
	}
}

func (p *program) Stop(s service.Service) error {
	return nil
}

func main() {
	s, _ := service.New(&program{}, &service.Config{
		Name:        "LocalTime",
		DisplayName: "Local Time Keeper",
		Description: "Cập nhật thời gian hệ thống theo múi giờ Asia/Ho_Chi_Minh",
	})

	if len(os.Args) > 1 {
		service.Control(s, os.Args[1])
		return
	}

	if service.Interactive() {
		fmt.Println("Đang cài đặt Windows Service...")
		if err := service.Control(s, "install"); err != nil {
			fmt.Println("Lỗi cài đặt (có thể do chưa chạy Run as Administrator hoặc đã cài rồi):", err)
		} else {
			fmt.Println("Cài đặt thành công!")
		}
		time.Sleep(10 * time.Second)
		fmt.Println("Đang khởi động Service...")
		if err := service.Control(s, "start"); err != nil {
			fmt.Println("Lỗi khởi động (hoặc service đang chạy rồi):", err)
		} else {
			fmt.Println("Khởi động thành công!")
		}

		fmt.Println("Xong! Cửa sổ sẽ tự đóng sau 5 giây.")
		time.Sleep(10 * time.Second)
		return
	}

	s.Run()
}

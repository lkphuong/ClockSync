package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/kardianos/service"

	_ "time/tzdata"
)

const (
	timeAPIURL         = "https://timeapi.io/api/time/current/zone?timeZone=Asia/Ho_Chi_Minh"
	timeAPITimeLayout  = "2006-01-02T15:04:05"
	systemLocationName = "Asia/Ho_Chi_Minh"

	// Định dạng theo Short Date mặc định của Windows tại Việt Nam (dd/MM/yyyy).
	// Nếu máy đích cấu hình Regional Settings khác, cần đổi lại cho khớp.
	systemDateLayout  = "02/01/2006"
	systemClockLayout = "15:04:05"

	syncInterval = 10 * time.Second

	logFileName = "localtime.log"
)

type timeAPIResponse struct {
	DateTime string `json:"dateTime"`
}

type logEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

type program struct {
	logger  *log.Logger
	logFile *os.File
}

// resolveLogFilePath trả về đường dẫn file log, đặt cạnh file thực thi để dễ tìm
// kể cả khi chương trình chạy dưới dạng Windows Service (không có thư mục làm việc cố định).
func resolveLogFilePath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("không xác định được đường dẫn thực thi: %w", err)
	}
	return filepath.Join(filepath.Dir(exePath), logFileName), nil
}

// newProgram khởi tạo program cùng file log, dùng chung cho cả chế độ chạy tương tác và Windows Service.
func newProgram() (*program, error) {
	logPath, err := resolveLogFilePath()
	if err != nil {
		return nil, err
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("không mở được file log %s: %w", logPath, err)
	}

	return &program{
		logger:  log.New(logFile, "", 0),
		logFile: logFile,
	}, nil
}

// writeLogEntry ghi một dòng log dạng JSON kèm timestamp, level và lỗi (nếu có).
func (p *program) writeLogEntry(level, message string, err error) {
	if p.logger == nil {
		return
	}

	entry := logEntry{
		Time:    time.Now().Format(time.RFC3339),
		Level:   level,
		Message: message,
	}
	if err != nil {
		entry.Error = err.Error()
	}

	data, marshalErr := json.Marshal(entry)
	if marshalErr != nil {
		return
	}
	p.logger.Println(string(data))
}

func (p *program) logInfo(message string) {
	p.writeLogEntry("INFO", message, nil)
}

func (p *program) logError(message string, err error) {
	p.writeLogEntry("ERROR", message, err)
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
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("không đặt được ngày hệ thống (%s): %w (output: %s)", dateValue, err, string(output))
	}
	return nil
}

// setSystemClock đặt giờ hệ thống Windows bằng lệnh cmd nội trú "time".
func (p *program) setSystemClock(t time.Time) error {
	timeValue := t.Format(systemClockLayout)
	cmd := exec.Command("cmd", "/C", fmt.Sprintf("time %s", timeValue))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("không đặt được giờ hệ thống (%s): %w (output: %s)", timeValue, err, string(output))
	}
	return nil
}

// updateSystemTime đồng bộ ngày giờ hệ thống theo thời gian chuẩn lấy từ timeapi.io,
// ghi log chi tiết ở từng bước để dễ chẩn đoán khi chạy dưới dạng Windows Service.
func (p *program) updateSystemTime() error {
	referenceTime, err := p.getTimeFromTimeAPI()
	if err != nil {
		p.logError("lấy giờ từ timeapi.io thất bại", err)
		return err
	}

	if err := p.setSystemDate(referenceTime); err != nil {
		p.logError("đặt ngày hệ thống thất bại", err)
		return err
	}

	if err := p.setSystemClock(referenceTime); err != nil {
		p.logError("đặt giờ hệ thống thất bại", err)
		return err
	}

	p.logInfo(fmt.Sprintf("cập nhật thành công, đồng bộ theo giờ %s", referenceTime.Format(time.RFC3339)))
	return nil
}

func (p *program) Start(s service.Service) error {
	p.logInfo("service đang khởi động")
	go p.run()
	return nil
}

func (p *program) run() {
	if err := p.updateSystemTime(); err != nil {
		p.logError("đồng bộ giờ lần đầu thất bại", err)
	}

	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()

	for range ticker.C {
		if err := p.updateSystemTime(); err != nil {
			p.logError("đồng bộ giờ định kỳ thất bại", err)
		}
	}
}

func (p *program) Stop(s service.Service) error {
	p.logInfo("service đang dừng")
	if p.logFile != nil {
		p.logFile.Close()
	}
	return nil
}

func main() {
	p, err := newProgram()
	if err != nil {
		fmt.Println("Không khởi tạo được chương trình:", err)
		os.Exit(1)
	}
	defer p.logFile.Close()

	s, err := service.New(p, &service.Config{
		Name:        "LocalTime",
		DisplayName: "Local Time Keeper",
		Description: "Cập nhật thời gian hệ thống theo múi giờ Asia/Ho_Chi_Minh",
	})
	if err != nil {
		p.logError("không tạo được service", err)
		fmt.Println("Không tạo được service:", err)
		os.Exit(1)
	}

	if len(os.Args) > 1 {
		if err := service.Control(s, os.Args[1]); err != nil {
			p.logError(fmt.Sprintf("lệnh service %q thất bại", os.Args[1]), err)
			fmt.Println("Lỗi điều khiển service:", err)
		}
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

	if err := s.Run(); err != nil {
		p.logError("service dừng do lỗi", err)
	}
}

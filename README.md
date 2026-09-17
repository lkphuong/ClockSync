# Hướng dẫn cài đặt ClockSync

## Yêu cầu

- Đã cài [NSSM](https://nssm.cc/) (Non-Sucking Service Manager)
- File `ClockSync.exe` có sẵn trên máy

## Bước 1: Cài đặt service

1. Mở **CMD với quyền Administrator**.
2. Chạy lệnh:

   ```cmd
   nssm install ClockSync
   ```

3. Cửa sổ cấu hình NSSM sẽ hiện ra:
   - Tab **Application**: trỏ **Path** đến file `ClockSync.exe`.
   - Tab **I/O**: chọn file `log.txt` cho cả **Output** và **Error** để lưu lại log của ClockSync.
4. Bấm **Install service** để hoàn tất.

## Bước 2: Khởi động service

Vẫn ở cửa sổ CMD, chạy:

```cmd
nssm start ClockSync
```

## Bước 3: Kiểm tra

Mở **Windows Services** (`services.msc`) và tìm `ClockSync` để xác nhận service đang ở trạng thái **Running**.

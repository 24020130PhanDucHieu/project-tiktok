@echo off
chcp 65001 >nul
echo ====================================================================
echo   KocFlow STUDIO - Khoi dong toan bo he thong tu dong song song
echo ====================================================================
echo.
echo [1/3] Khoi dong Zalo Bridge (Sidecar Port 5100)...
start "KocFlow - Zalo Bridge (:5100)" cmd /k "cd /d %~dp0zalo-bridge && npm start"

echo [2/3] Khoi dong Backend Go-Fiber (Port 5000)...
start "KocFlow - Backend Go (:5000)" cmd /k "cd /d %~dp0backend && go run ./cmd/api"

echo [3/3] Khoi dong Frontend React TypeScript (Port 5173)...
start "KocFlow - Frontend Studio (:5173)" cmd /k "cd /d %~dp0frontend && npm run dev"

echo.
echo --------------------------------------------------------------------
echo   DA KICH HOAT THANH CONG 3 DICH VU!
echo   * Giao dien Web:   http://localhost:5173
echo   * Go-Fiber API:   http://localhost:5000/health
echo   * Zalo Sidecar:   http://localhost:5100/health
echo --------------------------------------------------------------------


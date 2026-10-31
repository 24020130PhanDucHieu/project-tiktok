Write-Host "====================================================================" -ForegroundColor Cyan
Write-Host "  KocFlow STUDIO - Khoi dong toan bo he thong tu dong song song" -ForegroundColor Yellow
Write-Host "====================================================================" -ForegroundColor Cyan

Start-Process cmd -ArgumentList "/k cd /d `"$PSScriptRoot\zalo-bridge`" && npm start"
Start-Process cmd -ArgumentList "/k cd /d `"$PSScriptRoot\backend`" && go run ./cmd/api"
Start-Process cmd -ArgumentList "/k cd /d `"$PSScriptRoot\frontend`" && npm run dev"

Write-Host "Da khoi dong 3 dich vu!" -ForegroundColor Green
Write-Host "Web Studio:  http://localhost:5173" -ForegroundColor White
Write-Host "Go API:      http://localhost:5000/health" -ForegroundColor White
Write-Host "Zalo Bridge: http://localhost:5100/health" -ForegroundColor White


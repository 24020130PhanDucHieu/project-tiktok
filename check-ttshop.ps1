# Kiem tra ket noi TikTok Shop (CoolTech). Chay: .\check-ttshop.ps1
# Khong luu mat khau trong file nay, se hoi khi chay.

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $root
$api = "http://localhost:5000"

function Step($n, $text) { Write-Host "`n[$n] $text" -ForegroundColor Cyan }
function Ok($text)   { Write-Host "  OK  $text" -ForegroundColor Green }
function Bad($text)  { Write-Host "  LOI $text" -ForegroundColor Red }

# 1. Docker
Step 1 "Kiem tra Docker"
docker info *> $null
if ($LASTEXITCODE -ne 0) { Bad "Docker chua chay. Mo Docker Desktop, doi 1 phut roi chay lai file nay."; exit 1 }
Ok "Docker dang chay"

# 2. Bat container (khong xoa du lieu)
Step 2 "Bat cac container"
docker compose up -d | Out-Null
Ok "docker compose up -d xong"

# 3. Cho backend san sang
Step 3 "Cho backend (cong 5000)"
$ready = $false
for ($i = 0; $i -lt 30; $i++) {
    try { Invoke-RestMethod -Uri "$api/health" -TimeoutSec 3 | Out-Null; $ready = $true; break } catch { Start-Sleep -Seconds 2 }
}
if (-not $ready) { Bad "Backend khong tra loi. Xem log: docker compose logs --tail 30 backend"; exit 1 }
Ok "Backend dang song"

# 4. Dang nhap admin
Step 4 "Dang nhap admin"
$email = Read-Host "Email admin (Enter = admin@tiktoday.local)"
if (-not $email) { $email = "admin@tiktoday.local" }
$sec = Read-Host "Mat khau" -AsSecureString
$pw = [Runtime.InteropServices.Marshal]::PtrToStringAuto([Runtime.InteropServices.Marshal]::SecureStringToBSTR($sec))
try {
    $body = @{ email = $email; password = $pw } | ConvertTo-Json
    $res = Invoke-RestMethod -Method Post -Uri "$api/api/v1/auth/login" -ContentType "application/json" -Body $body
    $token = $res.data.token
} catch {
    Bad "Dang nhap that bai (sai email/mat khau?)"; exit 1
}
Ok "Dang nhap thanh cong"

# 5. Trang thai ket noi TikTok Shop (goi API that)
Step 5 "Goi /ttshop/status (hoi TikTok that)"
try {
    $st = (Invoke-RestMethod -Uri "$api/api/v1/ttshop/status" -Headers @{ Authorization = "Bearer $token" }).data
} catch {
    Bad "Khong goi duoc status: $($_.Exception.Message)"; exit 1
}
Write-Host "  configured : $($st.configured)"
Write-Host "  env        : $($st.env)"
Write-Host "  connected  : $($st.connected)"
if ($st.sellerName) { Write-Host "  seller     : $($st.sellerName)" }
if ($st.accessExpireAt) { Write-Host "  access het han: $($st.accessExpireAt)" }
foreach ($s in $st.shops) { Write-Host "  shop       : $($s.name) ($($s.region), id $($s.id))" }
if ($st.error) { Write-Host "  error      : $($st.error)" -ForegroundColor Yellow }

# 6. Kiem tra collection tts_tokens trong MongoDB (khong in token)
Step 6 "Kiem tra collection tts_tokens"
$js = 'var d=db.tts_tokens.find({}, {sellerName:1, accessExpireAt:1, refreshExpireAt:1, updatedAt:1}).toArray(); print("so ban ghi: " + d.length); d.forEach(function(x){ print("seller=" + x.sellerName + " | access het han=" + x.accessExpireAt + " | refresh het han=" + x.refreshExpireAt + " | cap nhat=" + x.updatedAt) })'
$js | docker compose exec -T mongodb mongosh --quiet tiktoday_booking

# Ket luan
Write-Host ""
if ($st.configured -and $st.connected -and $st.shops.Count -gt 0) {
    Write-Host "KET LUAN: Dang ket noi binh thuong voi $($st.shops[0].name)." -ForegroundColor Green
} elseif (-not $st.configured) {
    Write-Host "KET LUAN: Thieu khoa TTS trong container (kiem tra .env goc va docker-compose.yml)." -ForegroundColor Red
} else {
    Write-Host "KET LUAN: Chua ket noi. Can uy quyen lai shop (lay auth code moi)." -ForegroundColor Red
}

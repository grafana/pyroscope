# PowerShell port of ../rideshare/load-generator.py: continuously orders
# random vehicles from each region so the profiler has work to show.
$regions = @("us-east", "eu-north", "ap-south")
$vehicles = @("bike", "scooter", "car")

Write-Host ">> load generator started against: $($regions -join ', ')"
while ($true) {
    foreach ($region in $regions) {
        $vehicle = $vehicles | Get-Random
        try {
            Invoke-WebRequest -UseBasicParsing -TimeoutSec 30 "http://${region}:5000/$vehicle" | Out-Null
            Write-Host "ordered $vehicle from $region"
        } catch {
            Write-Host "failed to order $vehicle from ${region}: $($_.Exception.Message)"
        }
    }
    Start-Sleep -Milliseconds (Get-Random -Minimum 200 -Maximum 800)
}

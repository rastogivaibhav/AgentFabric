$ErrorActionPreference = 'Continue'
$checks = [ordered]@{
    'Enterprise agents' = 'http://localhost:8090/health'
    'AGS API'           = 'http://localhost:8000/health'
    'AGS dashboard'     = 'http://localhost:8080/'
    'PUL runtime'       = 'http://localhost:8082/health'
    'GovAgn portal'     = 'http://localhost:13000/'
    'GovAgn API'        = 'http://localhost:28081/healthz'
    'SPACE UI'          = 'http://localhost:3000/'
    'SPACE data plane'  = 'http://localhost:18091/health'
    'Ollama'            = 'http://localhost:11434/api/tags'
}
foreach ($entry in $checks.GetEnumerator()) {
    try {
        $response = Invoke-WebRequest -Uri $entry.Value -UseBasicParsing -TimeoutSec 5
        Write-Output ('{0,-19} PASS  HTTP {1}' -f $entry.Key, $response.StatusCode)
    } catch {
        Write-Output ('{0,-19} FAIL  unavailable' -f $entry.Key)
    }
}
try {
    $agents = Invoke-RestMethod -Uri 'http://localhost:8090/api/agents' -TimeoutSec 5
    foreach ($agent in $agents.agents) {
        Write-Output ("Agent: {0} | AGS: {1} | PUL: {2} | SPACE: {3}" -f $agent.name,$agent.ags_agent_id,$agent.pul_app_id,$agent.active_session.sandbox_id)
    }
} catch {}

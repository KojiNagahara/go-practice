$ErrorActionPreference = 'Stop'

$Profile = 'go-practice'
$Region = 'ap-northeast-1'
$StackName = 'go-practice-stack'
$Template = 'infrastructure/transformation/infra.yaml'
$Parameters = 'infrastructure/environments/dev-params.yaml'

if (-not (Get-Command aws -ErrorAction SilentlyContinue)) {
  throw 'AWS CLI was not found. Install AWS CLI v2 before deploying.'
}

if (-not (Test-Path $Template)) {
  throw "CloudFormation template was not found: $Template"
}

if (-not (Test-Path $Parameters)) {
  throw "Parameter file was not found: $Parameters"
}

$ParameterText = Get-Content -Raw $Parameters
if ($ParameterText -match 'Z1234567890ABC|replace-with-cert-arn|ami-0df7a6b6b9b3d5d24') {
  throw "Replace sample AWS values in $Parameters before deploying."
}

Write-Host "Checking AWS profile '$Profile'..."
aws sts get-caller-identity --profile $Profile --region $Region | Out-Null
if ($LASTEXITCODE -ne 0) {
  throw "AWS profile '$Profile' is not configured or cannot be authenticated."
}

Write-Host 'Validating CloudFormation template...'
& "$env:USERPROFILE\AppData\Roaming\Python\Python311\Scripts\cfn-lint.exe" $Template
if ($LASTEXITCODE -ne 0) {
    throw 'cfn-lint validation failed.'
}

Write-Host 'Deploying CloudFormation stack...'
aws cloudformation deploy `
  --profile $Profile `
  --region $Region `
  --stack-name $StackName `
  --template-file $Template `
  --parameter file://$Parameters `
  --capabilities CAPABILITY_IAM

if ($LASTEXITCODE -ne 0) {
    throw "AWS CloudFormation deployment failed for stack $StackName."
}

Write-Host 'Stack deployment complete.'
Write-Host 'You can verify outputs with:'
Write-Host "aws cloudformation describe-stacks --profile $Profile --region $Region --stack-name $StackName"

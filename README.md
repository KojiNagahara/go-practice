# Go Practice リポジトリ

このリポジトリは、バイブコーディングによるGo の学習用ワークスペースとして構成しています。

## 構成

- [src](src) : アプリケーションコードを置く領域
- [infrastructure](infrastructure) : AWS インフラ定義とデプロイ手順

## インフラ設計

本プロジェクトでは、AWS の CloudFormation を利用して以下の構成を定義しています。

- Route53 によるドメイン解決
- ALB を通した公開入口
- 複数 AZ に分散したアプリケーション層
- private subnet に配置した Aurora
- private subnet に配置した Redis
- private subnet の外向き通信用 NAT Gateway
- EC2 用 IAM ロールと Instance Profile

EC2 用 IAM ロールには、Systems Manager による管理権限と、Aurora の認証情報を Secrets Manager から取得するための権限を設定しています。

## インフラ関連ファイル

- [infrastructure/transformation/infra.yaml](infrastructure/transformation/infra.yaml) : CloudFormation テンプレート
- [infrastructure/environments/dev-params.yaml](infrastructure/environments/dev-params.yaml) : 環境別の値
- [infrastructure/secrets/dev-secrets.yaml](infrastructure/secrets/dev-secrets.yaml) : サンプル秘密情報
- [infrastructure/deploy.ps1](infrastructure/deploy.ps1) : デプロイスクリプト
- [infrastructure/README.md](infrastructure/README.md) : デプロイ手順書

## ローカル検証

CloudFormation の lint を実行します。

```powershell
& "$env:USERPROFILE\AppData\Roaming\Python\Python311\Scripts\cfn-lint.exe" "infrastructure/transformation/infra.yaml"
```

## デプロイ

```powershell
powershell -ExecutionPolicy Bypass -File .\infrastructure\deploy.ps1
```

実際の運用では、以下の値を実環境向けに置き換えてください。

- Route53 Hosted Zone ID
- ACM 証明書 ARN
- AMI ID
- Aurora の秘密情報
- AWS アカウント / リージョン

## 注意事項

- 秘密情報は Git 管理しないことを推奨します。
- 本番環境では Secrets Manager や SSM Parameter Store の利用を推奨します。
- 実デプロイ前に AWS CLI の認証が完了していることを確認してください。

# AWS インフラ構成仕様

## 目的

Go アプリケーションを AWS 上で実行するための、複数 AZ 構成の CloudFormation インフラを定義する。
公開入口は `https://www.kjnghr.com/` とし、アプリケーション、データベース、セッションストアをネットワークと IAM の境界で分離する。

## 構成要件

- Route53 で `www.kjnghr.com` を ALB へ Alias 接続する
- ACM 証明書を ALB の HTTPS リスナーへ設定する
- HTTP は HTTPS へリダイレクトする
- VPC は `10.0.0.0/16` とする
- 2 つの AZ に以下のサブネットを作成する
	- Public subnet: ALB、NAT Gateway
	- Private application subnet: Go アプリケーション用 EC2
	- Private data subnet: Aurora、Redis
- アプリケーションサーバーは Auto Scaling Group で 2 台以上にする
- ALB でアプリケーションサーバーへ負荷分散する
- データベースは Aurora MySQL とし、2 AZ に DB インスタンスを配置する
- セッションデータは Redis Replication Group で管理する
- Aurora と Redis は Public subnet に配置しない

## NAT Gateway の目的

Private subnet の EC2 はインターネットから直接アクセスできない。一方で、OS 更新、パッケージ取得、コンテナイメージ取得などの外向き通信は必要になるため、Public subnet に NAT Gateway を配置する。

Private subnet のデフォルトルートを NAT Gateway へ向け、戻り通信を許可する。インターネットから Private subnet への着信経路は作成しない。

## セキュリティグループ

- ALB: インターネットから HTTP / HTTPS を許可
- Application: ALB のセキュリティグループからアプリケーションポートのみ許可
- Database: Application のセキュリティグループから MySQL ポートのみ許可
- Redis: Application のセキュリティグループから Redis ポートのみ許可

## IAM 設計

CloudFormation でアプリケーション用の IAM リソースを自動作成する。

### EC2 用 IAM

- `AppInstanceRole`
	- EC2 サービスだけが AssumeRole できる
	- `AmazonSSMManagedInstanceCore` で Systems Manager による管理を許可する
	- Aurora の Secrets Manager シークレットに対する `DescribeSecret` / `GetSecretValue` のみ許可する
	- `Project`、`Environment`、`ManagedBy` タグを付与する
- `AppInstanceProfile`
	- `AppInstanceRole` を EC2 に割り当てる

ロール名は `${AWS::StackName}-app-instance-role`、Instance Profile 名は `${AWS::StackName}-app-instance-profile` とする。これにより、デプロイ実行者の `iam:PassRole` を対象ロールに限定できる。

### デプロイ実行者用 IAM

[../iam/deployer-passrole-policy.json](../iam/deployer-passrole-policy.json) で、以下の条件を付けた `iam:PassRole` を付与する。

- 対象: アプリケーション用 IAM ロールのみ
- 渡せるサービス: `ec2.amazonaws.com` のみ

また、デプロイ実行者には CloudFormation、VPC、EC2、ALB、Auto Scaling、RDS、ElastiCache、Route53、Secrets Manager、CloudWatch、IAM の必要な操作権限が必要である。

## 秘密情報

Aurora のマスターパスワードは `AWS::SecretsManager::Secret` の `GenerateSecretString` で AWS 側に自動生成する。

- パスワードをテンプレートや Git に直接記載しない
- Aurora は Secrets Manager の動的参照を利用する
- アプリケーションからの参照は IAM ロールで対象シークレットだけに制限する
- `secrets/dev-secrets.yaml` はサンプルであり、実際の秘密情報を記載しない

## ファイル構成

- [../transformation/infra.yaml](../transformation/infra.yaml): CloudFormation の構造定義
- [../environments/dev-params.yaml](../environments/dev-params.yaml): 環境固有のパラメータ
- [../secrets/dev-secrets.yaml](../secrets/dev-secrets.yaml): 秘密情報のサンプルと注意事項
- [../iam/deployer-passrole-policy.json](../iam/deployer-passrole-policy.json): デプロイ実行者用の限定 PassRole ポリシー
- [../deploy.ps1](../deploy.ps1): lint と CloudFormation deploy を実行する PowerShell スクリプト
- [../../README.md](../../README.md): リポジトリ概要
- [../README.md](../README.md): インフラのデプロイ手順

## 環境固有値

[../environments/dev-params.yaml](../environments/dev-params.yaml) に以下を定義する。

- ドメイン名
- Route53 Hosted Zone ID
- ACM 証明書 ARN
- アプリケーション用 AMI ID
- EC2、Aurora、Redis のインスタンスタイプ

Hosted Zone ID、AMI ID、証明書 ARN は AWS アカウントとリージョンに依存するため、サンプル値を実値へ置き換えてからデプロイする。

## 検証とデプロイ

### CloudFormation lint

```powershell
& "$env:USERPROFILE\AppData\Roaming\Python\Python311\Scripts\cfn-lint.exe" "infrastructure/transformation/infra.yaml"
```

### AWS 認証確認

```powershell
aws configure --profile go-practice
aws sts get-caller-identity --profile go-practice --region ap-northeast-1
```

### デプロイ

```powershell
powershell -ExecutionPolicy Bypass -File .\infrastructure\deploy.ps1
```

デプロイスクリプトは、AWS CLI、テンプレート、パラメータファイル、サンプル値、AWS プロファイルを事前確認する。CloudFormation の実行には `CAPABILITY_IAM` を指定する。

## 現在の状態

- CloudFormation テンプレート作成済み
- 環境値とテンプレートを分離済み
- Secrets Manager による Aurora パスワード自動生成を実装済み
- IAM ロール、Instance Profile、PassRole 制限を実装済み
- cfn-lint 検証済み
- AWS 認証情報と実環境固有値が未設定の場合、実デプロイは実行しない
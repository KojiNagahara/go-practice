# インフラデプロイ手順

このフォルダには、Go アプリケーション用の AWS CloudFormation テンプレートとデプロイ手順が含まれています。

## ファイル一覧

- [transformation/infra.yaml](transformation/infra.yaml) : CloudFormation テンプレート
- [environments/dev-params.yaml](environments/dev-params.yaml) : 環境依存の値
- [secrets/dev-secrets.yaml](secrets/dev-secrets.yaml) : 秘密情報のサンプル
- [deploy.ps1](deploy.ps1) : AWS CLI 用デプロイスクリプト

## IAM の自動作成

CloudFormation により、アプリケーション用 EC2 インスタンス向けに以下の IAM リソースを自動作成します。

- `AppInstanceRole` : Systems Manager 接続権限と Aurora の Secrets Manager 読み取り権限を持つロール
- `AppInstanceProfile` : EC2 にロールを割り当てる Instance Profile
- `iam/deployer-passrole-policy.json` : デプロイ実行者向けの限定的な `iam:PassRole` ポリシー

`AppInstanceRole` と `AppInstanceProfile` は `${AWS::StackName}` を含む名前で作成されます。`iam/deployer-passrole-policy.json` の `<AWS_ACCOUNT_ID>` を実際の AWS アカウント ID に置き換えて、デプロイ実行者へ付与してください。`iam:PassRole` は EC2 サービスに対してアプリケーション用ロールだけを渡せる設定です。

デプロイ実行者には、上記に加えて IAM ロールと Instance Profile の作成・削除・更新権限が必要です。テンプレートにカスタム名付き IAM リソースがあるため、デプロイ時の `CAPABILITY_IAM` で許可できます。

## 前提条件

- AWS CLI v2 がインストール済みであること
- AWS 認証情報が設定済みであること
- IAM 権限に VPC / EC2 / ALB / ACM / RDS / ElastiCache / S3 / Route53 / Secrets Manager / CloudWatch / IAM が含まれていること
- CloudFormation 実行者が Route 53 ホストゾーン、ACM 証明書、DNS検証レコードを作成できること
- ドメインレジストラ側で、デプロイ後に出力されるNameServersを設定できること

## 1. AWS 認証情報の設定

```powershell
aws configure --profile go-practice
aws sts get-caller-identity --profile go-practice
```

## 2. 環境値の更新

[environments/dev-params.yaml](environments/dev-params.yaml) を編集して、実 AWS 環境に合う値へ置き換えてください。

必須項目:
- DomainName
- AppDomainName
- AmiId

サンプル:

```yaml
Parameters:
  DomainName: kjnghr.com
  AppDomainName: www
  AmiId: ami-0df7a6b6b9b3d5d24
  AppInstanceType: t3.small
  DatabaseInstanceClass: db.t3.medium
  RedisNodeType: cache.t3.micro
```

## 3. CloudFormation テンプレートの lint 検証

```powershell
& "$env:USERPROFILE\AppData\Roaming\Python\Python311\Scripts\cfn-lint.exe" "infrastructure/transformation/infra.yaml"
```

## 4. スタックのデプロイ

デプロイスクリプトを実行します。

```powershell
powershell -ExecutionPolicy Bypass -File .\infrastructure\deploy.ps1
```

このスクリプトの中では、以下を実行します。

```powershell
aws cloudformation deploy `
  --profile go-practice `
  --region ap-northeast-1 `
  --stack-name go-practice-stack `
  --template-file infrastructure/transformation/infra.yaml `
  --parameter file://infrastructure/environments/dev-params.yaml `
  --capabilities CAPABILITY_IAM
```

## 5. デプロイ結果の確認

```powershell
aws cloudformation describe-stacks `
  --profile go-practice `
  --region ap-northeast-1 `
  --stack-name go-practice-stack
```

出力で確認する項目:
- AppUrl
- AlbDnsName
- NameServers
- AuroraEndpoint
- RedisEndpoint
- ImageUploadBucketName

## 6. デプロイ後の確認ポイント

- ALB のヘルスチェックが正常であること
- Aurora クラスタが利用可能であること
- Route53 レコードが作成されていること
- https://www.kjnghr.com/ にアクセスできること

## 注意事項

- 秘密情報は Git 管理しないことを推奨します
- 本番では Secrets Manager または SSM Parameter Store の利用を推奨します
- ACM 証明書はCloudFormationが作成し、Route 53 DNS検証レコードも自動登録します
- Route 53 ホストゾーンもCloudFormationが作成します。デプロイ後、出力のNameServersをドメインレジストラに設定してください
- ACM証明書はALBと同じリージョンで作成されます
- 画像アップロード用S3バケットは非公開、SSE-S3暗号化、バージョニング有効で作成されます
- アプリケーションEC2には画像オブジェクトの読み書き・削除とバケット一覧の最小権限を付与します

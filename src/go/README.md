# Go Web アプリケーション

AWS CloudFormation で構築する ALB 配下のアプリケーションサーバー用 Web アプリケーションです。

## 必要な環境

- Go `1.27.1` 以降
- Docker を使う場合は Docker Engine または Docker Desktop

## 起動

```powershell
go run .\cmd\web
```

既定では `8080` ポートで起動します。

## エンドポイント

- `GET /` : アプリケーションの稼働情報
- `GET /health` : ALB のヘルスチェック用。常に HTTP 200 を返す
- `GET /ready` : DB と Redis の接続先設定状態
- `GET /categories` : カテゴリ管理画面
- `GET /api/categories` : カテゴリ一覧
- `POST /api/categories` : カテゴリ作成（`{"name":"カテゴリ名"}`）
- `GET /api/categories/{id}` : カテゴリ取得
- `PUT/PATCH /api/categories/{id}` : カテゴリ更新（`{"name":"カテゴリ名"}`）
- `DELETE /api/categories/{id}` : カテゴリ削除
- `GET /items` : 商品・商品画像を同時登録できる商品管理画面
- `GET /item-images` : 商品画像管理画面
- 一覧画面（カテゴリ・商品・商品画像）は既定10件のページネーションに対応し、画面上の表示件数セレクターで10/20/50/100件へ変更できます。
- `GET /api/items` : 商品一覧
- `POST /api/items` : 商品作成（`{"name":"商品名","description":"説明","price":1000,"image_urls":["https://..."]}`）。価格は従来どおり小数の通貨単位で指定し、内部では整数の最小単位で保存
- `GET /api/items/{id}` : 商品取得
- `PUT/PATCH /api/items/{id}` : 商品更新
- `DELETE /api/items/{id}` : 商品削除
- `GET /api/item-images` : 商品画像一覧
- `POST /api/item-images` : 商品画像作成（`{"item_id":1,"image_url":"https://..."}`）
- `GET /api/item-images/{id}` : 商品画像取得
- `PUT/PATCH /api/item-images/{id}` : 商品画像更新
- `DELETE /api/item-images/{id}` : 商品画像削除

`/items/{id}/edit` では、商品情報の更新、既存画像の削除、新しい画像の追加を同じ画面で実行できます。

## 環境変数

| 変数 | 既定値 | 用途 |
| --- | --- | --- |
| `PORT` | `8080` | HTTP 待受ポート。CloudFormation の ALB Target Group と一致させる |
| `APP_ENV` | `development` | 実行環境名 |
| `DATABASE_URL` | 空 | MySQL DSN（例: `user:password@tcp(host:3306)/database?parseTime=true`）。認証情報はログへ出さない |
| `REDIS_URL` | 空 | Redis 接続先。認証情報はログへ出さない |
| `STORAGE_DRIVER` | `local` | 画像保存先。`local` または `s3` |
| `UPLOAD_DIR` | `/data/uploads` | `local` 使用時の保存ディレクトリ |
| `S3_BUCKET` | 空 | `s3` 使用時のS3バケット名 |
| `GOOGLE_OAUTH_CLIENT_ID` / `GOOGLE_OAUTH_CLIENT_SECRET` | 空 | Google OAuth 2.0クライアント情報 |
| `GOOGLE_OAUTH_REDIRECT_URL` | 空 | Google callback URL（例: `http://localhost:8081/oauth/google/callback`） |
| `AMAZON_OAUTH_CLIENT_ID` / `AMAZON_OAUTH_CLIENT_SECRET` | 空 | Amazon OAuth 2.0クライアント情報 |
| `AMAZON_OAUTH_REDIRECT_URL` | 空 | Amazon callback URL（例: `http://localhost:8081/oauth/amazon/callback`） |
| `HTTP_READ_HEADER_TIMEOUT` | `5s` | HTTPリクエストヘッダーの読み込みタイムアウト |
| `HTTP_READ_TIMEOUT` | `10s` | HTTPリクエスト全体の読み込みタイムアウト |
| `HTTP_WRITE_TIMEOUT` | `10s` | HTTPレスポンスの書き込みタイムアウト |
| `HTTP_IDLE_TIMEOUT` | `60s` | Keep-Alive接続のアイドルタイムアウト |
| `HTTP_SHUTDOWN_TIMEOUT` | `10s` | Graceful Shutdownの最大待機時間 |

時間値はGoのduration形式（例: `5s`, `1m`）で指定します。未設定または不正な値の場合は既定値を使用します。

`USERS` テーブルのユーザーで `/login` に認証します。ログインセッションはRedisに保存されます。一般ユーザーは `/shop` 配下のみ、管理ユーザーは管理画面・APIを含む全画面へアクセスできます。初期SQLの開発用アカウントは `admin/admin_password` と `user/user_password` です。

`/register` から一般ユーザーを登録できます。Google/AmazonのOAuth 2.0ログインは、各プロバイダーでcallback URLを登録し、上記の環境変数を設定した場合に有効になります。OAuthユーザーはプロバイダーとsubject IDでRDB上のアカウントに紐付けられます。

状態変更リクエストは同一オリジン検証または `X-CSRF-Token` によるCSRF検証を行い、ログイン失敗はIPアドレスとユーザー名ごとに15分間で5回までに制限します。本番環境（`APP_ENV` が `local` / `development` 以外）ではセッションCookieとCSRF Cookieに `Secure` 属性を付与します。

Aurora のパスワードは AWS Secrets Manager で管理し、アプリケーションへは安全な実行時設定として渡します。ソースコードやイメージへ秘密情報を埋め込まないでください。

カテゴリ管理を利用するには、`CATEGORIES` テーブルを含む [item.sql](../sql/item.sql) をAuroraへ適用し、`DATABASE_URL` を設定してください。

## アプリケーションの構成

カテゴリ機能は次の3層に分離しています。

- `internal/httpapi/categories.go` : HTTPリクエスト、レスポンス、画面描画
- `internal/categories/category.go` : カテゴリの業務ルールと入力検証
- `internal/categories/repository.go` : Aurora MySQLへのSQLアクセス

依存方向は `Handler -> Service -> Repository` とし、SQLや業務ルールがHTTPハンドラーへ直接混在しない構成にしています。`UpdateItem` が商品情報と画像変更を一つの集約ユースケースとして扱います。

商品・商品画像機能も同じ構成です。

- `internal/httpapi/items.go` : 商品・画像のHTTP APIと画面描画
- `internal/items/item.go` : 商品集約（画像を含む）、Money値オブジェクト、更新ユースケース
- `internal/items/repository.go` : `ITEMS`・`ITEM_IMAGES`へのSQLアクセス

## テスト

```powershell
go test ./...
```

## Docker

Docker Composeで起動すると、Goアプリケーションは`go-sample-docker`、MySQLは`go-sample-mysql`、Redisは`go-sample-redis`というコンテナ名で実行されます。Compose定義は [`../docker/docker-compose.yml`](../docker/docker-compose.yml) にあり、MySQLの初回起動時に、`../sql/item.sql`を実行してテーブルとテストデータを作成します。

商品画面からの画像ファイルは、開発環境では`upload-data`ボリュームへ保存され、`/uploads/`から配信されます。本番環境で`STORAGE_DRIVER=s3`を指定すると、AWS SDKの標準認証チェーンで非公開S3バケットへ保存します。DBには不変のオブジェクトキーを保存し、画面/APIの読み出し時だけ1時間有効の署名付きGET URLへ解決します。ローカル環境では同じキーを`/uploads/` URLへ解決します。

既存DBを更新する場合は、[001_priority_a.sql](../sql/migrations/001_priority_a.sql)を一度適用してください。Dockerの新規環境では`docker compose down -v`後の起動で最新の`item.sql`が適用されます。

```powershell
Set-Location ../docker
docker compose up --build -d
Invoke-WebRequest http://localhost:8081/health
Invoke-WebRequest http://localhost:8081/categories
```

停止する場合:

```powershell
Set-Location ../docker
docker compose down
```

MySQLのデータを初期化し直す場合は、コンテナとボリュームを削除してから再起動します。

```powershell
Set-Location ../docker
docker compose down -v
docker compose up --build -d
```

`/categories` はCompose内のMySQLへ接続して、`CATEGORIES`のデータを表示します。Redisは`redis://redis:6379`でGoコンテナから接続でき、`/ready`でもRedis設定済みとして扱われます。

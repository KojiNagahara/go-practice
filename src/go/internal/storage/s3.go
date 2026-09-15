package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3 struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

func NewS3(client *s3.Client, bucket string) *S3 {
	return &S3{client: client, presign: s3.NewPresignClient(client), bucket: bucket}
}

func (s3Storage *S3) Put(ctx context.Context, object Object) (string, error) {
	key := SafeKey(object.Name)
	_, err := s3Storage.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s3Storage.bucket),
		Key:         aws.String(key),
		Body:        object.Reader,
		ContentType: aws.String(object.ContentType),
	})
	if err != nil {
		return "", err
	}
	return key, nil
}

func (s3Storage *S3) ResolveURL(ctx context.Context, key string) (string, error) {
	if key == "" || s3Storage.presign == nil {
		return "", fmt.Errorf("S3 storage is not configured")
	}
	presigned, err := s3Storage.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s3Storage.bucket),
		Key:    aws.String(key),
	}, func(options *s3.PresignOptions) {
		options.Expires = time.Hour
	})
	if err != nil {
		return "", err
	}
	return presigned.URL, nil
}

func (s3Storage *S3) Delete(ctx context.Context, key string) error {
	if key == "" {
		return nil
	}
	_, err := s3Storage.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s3Storage.bucket),
		Key:    aws.String(key),
	})
	return err
}

func NewS3ObjectName(itemID uint64, original string) string {
	return fmt.Sprintf("items/%d/%s", itemID, SafeName(original))
}

module github.com/scttfrdmn/cargoship/examples/library-usage/01-simple-upload

go 1.27.0

replace github.com/scttfrdmn/cargoship => ../../..

require github.com/scttfrdmn/cargoship v0.0.0-00010101000000-000000000000

require (
	github.com/aws/aws-sdk-go-v2 v1.47.3 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.22 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.5.6 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.8.6 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.5.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.21 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.11.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.14.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.20.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/kms v1.61.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/s3 v1.114.3 // indirect
	github.com/aws/smithy-go v1.28.5 // indirect
	github.com/klauspost/compress v1.20.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

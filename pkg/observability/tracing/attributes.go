// Package tracing provides OpenTelemetry distributed tracing for CargoShip
package tracing

import "go.opentelemetry.io/otel/attribute"

// Standard OpenTelemetry span attributes for CargoShip operations
const (
	// Upload tracking
	AttrUploadID      = attribute.Key("cargoship.upload_id")
	AttrPipelineStage = attribute.Key("cargoship.pipeline.stage")
	AttrJobID         = attribute.Key("cargoship.job.id")
	AttrShardID       = attribute.Key("cargoship.shard.id")
	AttrChunkID       = attribute.Key("cargoship.chunk.id")
	AttrAttempt       = attribute.Key("cargoship.attempt")

	// File attributes
	AttrFileName  = attribute.Key("file.name")
	AttrFileSize  = attribute.Key("file.size")
	AttrFileCount = attribute.Key("file.count")

	// S3 attributes
	AttrS3Bucket       = attribute.Key("aws.s3.bucket")
	AttrS3Key          = attribute.Key("aws.s3.key")
	AttrS3Region       = attribute.Key("aws.s3.region")
	AttrS3StorageClass = attribute.Key("aws.s3.storage_class")
	AttrS3Prefix       = attribute.Key("aws.s3.prefix")

	// Network attributes
	AttrBandwidth     = attribute.Key("network.bandwidth")
	AttrRTT           = attribute.Key("network.rtt")
	AttrTransporter   = attribute.Key("cargoship.transporter")
	AttrCongestionWin = attribute.Key("cargoship.congestion_window")
)

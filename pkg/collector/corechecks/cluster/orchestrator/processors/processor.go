// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator
// +build orchestrator

package processors

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	jsoniter "github.com/json-iterator/go"
	"k8s.io/apimachinery/pkg/types"
)

// ProcessorContext holds resource processing attributes
type ProcessorContext struct {
	APIClient  *apiserver.APIClient
	Cfg        *config.OrchestratorConfig
	ClusterID  string
	HostName   string
	MsgGroupID int32
	NodeType   orchestrator.NodeType
}

// Handlers is the interface that is to be implemented for every resource type
// and provides a way to plug in code at different levels of the Processor
// logic.
type Handlers interface {
	// AfterMarshalling runs before the Processor marshals the resource to
	// generate a manifest. If skip is true then the resource processing loop
	// moves on to the next resource.
	AfterMarshalling(ctx *ProcessorContext, resource, resourceModel interface{}, yaml []byte) (skip bool)

	// BeforeCacheCheck runs before the Processor does a cache lookup for the
	// resource. If skip is true then the resource processing loop moves on to
	// the next resource.
	BeforeCacheCheck(ctx *ProcessorContext, resource, resourceModel interface{}) (skip bool)

	// BeforeMarshalling runs before the Processor marshals the resource to
	// generate a manifest. If skip is true then the resource processing loop
	// moves on to the next resource.
	BeforeMarshalling(ctx *ProcessorContext, resource, resourceModel interface{}) (skip bool)

	// BuildMessageBody is used to build a message containing a chunk of
	// resource models of a certain size. If skip is true then the resource
	// processing loop moves on to the next resource.
	BuildMessageBody(ctx *ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody

	// ExtractResource is used to build the a resource model from the raw
	// resource representation.
	ExtractResource(ctx *ProcessorContext, resource interface{}) (resourceModel interface{})

	// ResourceList is used to convert a list of raw resources to a generic list
	// that can be used throughout a Processor.
	ResourceList(ctx *ProcessorContext, list interface{}) (resources []interface{})

	// ResourceUID returns the resource UID.
	ResourceUID(ctx *ProcessorContext, resource, resourceModel interface{}) types.UID

	// ResourceVersion returns the resource Version.
	ResourceVersion(ctx *ProcessorContext, resource, resourceModel interface{}) string

	// ScrubBeforeExtraction replaces sensitive information in the resource
	// before resource extraction.
	ScrubBeforeExtraction(ctx *ProcessorContext, resource interface{})

	// ScrubBeforeMarshalling replaces sensitive information in the resource
	// before resource marshalling.
	ScrubBeforeMarshalling(ctx *ProcessorContext, resource interface{})

	// BuildManifestMessageBody is used to build a message containing a chunk of
	// manifest models of a certain size.
	BuildManifestMessageBody(ctx *ProcessorContext, resourceManifests []interface{}, groupSize int) model.MessageBody
}

// Processor is a generic resource processing component. It relies on a set of
// handlers to enrich its processing logic and make it a processor for resources
// of a specific type.
type Processor struct {
	h Handlers
}

// ProcessResult contains the result of metadata and manifest
type ProcessResult struct {
	MetadataMessages []model.MessageBody
	ManifestMessages []model.MessageBody
}

// NewProcessor creates a new processor for a resource type.
func NewProcessor(h Handlers) *Processor {
	return &Processor{
		h: h,
	}
}

// Process is used to process a list of resources of a certain type.
func (p *Processor) Process(ctx *ProcessorContext, list interface{}) (processResult ProcessResult, processed int) {
	// This default allows detection of panic recoveries.
	processed = -1

	// Make sure to recover if a panic occurs.
	defer RecoverOnPanic()

	resourceList := p.h.ResourceList(ctx, list)
	resourceMetadataModels := make([]interface{}, 0, len(resourceList))
	resourceManifestModels := make([]interface{}, 0, len(resourceList))

	for _, resource := range resourceList {
		// Scrub before extraction.
		p.h.ScrubBeforeExtraction(ctx, resource)

		// Extract the message model from the resource.
		resourceMetadataModel := p.h.ExtractResource(ctx, resource)

		// Execute code before cache check.
		if skip := p.h.BeforeCacheCheck(ctx, resource, resourceMetadataModel); skip {
			continue
		}

		// Cache check
		resourceUID := p.h.ResourceUID(ctx, resource, resourceMetadataModel)
		resourceVersion := p.h.ResourceVersion(ctx, resource, resourceMetadataModel)

		if orchestrator.SkipKubernetesResource(resourceUID, resourceVersion, ctx.NodeType) {
			continue
		}

		// Execute code before marshalling.
		if skip := p.h.BeforeMarshalling(ctx, resource, resourceMetadataModel); skip {
			continue
		}

		// Scrub the resource before marshalling.
		p.h.ScrubBeforeMarshalling(ctx, resource)

		// Marshal the resource to generate the YAML field.
		yaml, err := jsoniter.Marshal(resource)
		if err != nil {
			log.Warnf(newMarshallingError(err).Error())
			continue
		}

		// Execute code after marshalling.
		if skip := p.h.AfterMarshalling(ctx, resource, resourceMetadataModel, yaml); skip {
			continue
		}

		resourceMetadataModels = append(resourceMetadataModels, resourceMetadataModel)

		// Add resource manifest
		resourceManifestModels = append(resourceManifestModels, &model.Manifest{
			Type:            int32(ctx.NodeType.CollectorType()),
			Uid:             string(resourceUID),
			ResourceVersion: resourceVersion,
			Content:         yaml,
			Version:         "v1",
			ContentType:     "json",
		})
	}
	// Split messages in chunks
	chunkCount := orchestrator.GroupSize(len(resourceMetadataModels), ctx.Cfg.MaxPerMessage)

	// chunk orchestrator metadata and manifest
	metadataChunks := chunkResources(resourceMetadataModels, chunkCount, ctx.Cfg.MaxPerMessage)
	manifestChunks := chunkResources(resourceManifestModels, chunkCount, ctx.Cfg.MaxPerMessage)

	metadataMessages := make([]model.MessageBody, 0, chunkCount)
	manifestMessages := make([]model.MessageBody, 0, chunkCount)
	for i := 0; i < chunkCount; i++ {
		metadataMessages = append(metadataMessages, p.h.BuildMessageBody(ctx, metadataChunks[i], chunkCount))
		manifestMessages = append(manifestMessages, p.h.BuildManifestMessageBody(ctx, manifestChunks[i], chunkCount))
	}
	processResult = ProcessResult{
		MetadataMessages: metadataMessages,
		ManifestMessages: manifestMessages,
	}
	return processResult, len(resourceMetadataModels)
}

// chunkResources splits messages into groups of messages called chunks, knowing
// the expected chunk count and size.
func chunkResources(resources []interface{}, chunkCount, chunkSize int) [][]interface{} {
	chunks := make([][]interface{}, 0, chunkCount)

	for counter := 1; counter <= chunkCount; counter++ {
		chunkStart, chunkEnd := orchestrator.ChunkRange(len(resources), chunkCount, chunkSize, counter)
		chunks = append(chunks, resources[chunkStart:chunkEnd])
	}

	return chunks
}

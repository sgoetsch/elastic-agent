// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package otel

import (
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/receiver"

	// Receivers:
	filelogreceiver "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/filelogreceiver" // for collecting log files
	hostmetricsreceiver "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver"
	httpcheckreceiver "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/httpcheckreceiver"
	jaegerreceiver "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/jaegerreceiver"
	jmxreceiver "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/jmxreceiver"
	k8sclusterreceiver "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/k8sclusterreceiver"
	k8sobjectsreceiver "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/k8sobjectsreceiver"
	kubeletstatsreceiver "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/kubeletstatsreceiver"
	nginxreceiver "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/nginxreceiver"
	receivercreator "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/receivercreator"
	redisreceiver "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/redisreceiver"
	zipkinreceiver "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/zipkinreceiver"
	nopreceiver "go.opentelemetry.io/collector/receiver/nopreceiver"
	otlpreceiver "go.opentelemetry.io/collector/receiver/otlpreceiver"

	elasticapmintakereceiver "github.com/elastic/opentelemetry-collector-components/receiver/elasticapmintakereceiver" // for collecting APM data from Elastic APM agents

	fbreceiver "github.com/elastic/beats/v7/x-pack/filebeat/fbreceiver"
	mbreceiver "github.com/elastic/beats/v7/x-pack/metricbeat/mbreceiver"

	// Processors:
	attributesprocessor "github.com/open-telemetry/opentelemetry-collector-contrib/processor/attributesprocessor" // for modifying signal attributes
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/cumulativetodeltaprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/filterprocessor"
	geoipprocessor "github.com/open-telemetry/opentelemetry-collector-contrib/processor/geoipprocessor"                 // for adding geographical metadata associated to an IP address
	k8sattributesprocessor "github.com/open-telemetry/opentelemetry-collector-contrib/processor/k8sattributesprocessor" // for adding k8s metadata
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourcedetectionprocessor"
	resourceprocessor "github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourceprocessor"   // for modifying resource attributes
	transformprocessor "github.com/open-telemetry/opentelemetry-collector-contrib/processor/transformprocessor" // for OTTL processing on logs
	"go.opentelemetry.io/collector/processor/batchprocessor"                                                    // for batching events
	"go.opentelemetry.io/collector/processor/memorylimiterprocessor"

	"github.com/elastic/opentelemetry-collector-components/processor/elastictraceprocessor"

	"github.com/elastic/opentelemetry-collector-components/processor/elasticinframetricsprocessor"

	// Exporters:
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/elasticsearchexporter"
	fileexporter "github.com/open-telemetry/opentelemetry-collector-contrib/exporter/fileexporter" // for e2e tests
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/loadbalancingexporter"
	debugexporter "go.opentelemetry.io/collector/exporter/debugexporter" // for dev
	nopexporter "go.opentelemetry.io/collector/exporter/nopexporter"
	"go.opentelemetry.io/collector/exporter/otlpexporter"
	otlphttpexporter "go.opentelemetry.io/collector/exporter/otlphttpexporter"

	// Extensions
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/bearertokenauthextension"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension"
	healthcheckv2extension "github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckv2extension"
	k8sobserver "github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer/k8sobserver"
	pprofextension "github.com/open-telemetry/opentelemetry-collector-contrib/extension/pprofextension"
	filestorage "github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/filestorage"
	"go.opentelemetry.io/collector/extension/memorylimiterextension" // for putting backpressure when approach a memory limit

	"github.com/elastic/opentelemetry-collector-components/extension/apikeyauthextension"
	"github.com/elastic/opentelemetry-collector-components/extension/apmconfigextension"

	// Connectors
	routingconnector "github.com/open-telemetry/opentelemetry-collector-contrib/connector/routingconnector"
	spanmetricsconnector "github.com/open-telemetry/opentelemetry-collector-contrib/connector/spanmetricsconnector"
	forwardconnector "go.opentelemetry.io/collector/connector/forwardconnector"

	elasticapmconnector "github.com/elastic/opentelemetry-collector-components/connector/elasticapmconnector"
)

func components(extensionFactories ...extension.Factory) func() (otelcol.Factories, error) {
	return func() (otelcol.Factories, error) {
		var err error
		factories := otelcol.Factories{}

		// Receivers
		receivers := []receiver.Factory{
			elasticapmintakereceiver.NewFactory(),
			otlpreceiver.NewFactory(),
			filelogreceiver.NewFactory(),
			kubeletstatsreceiver.NewFactory(),
			k8sclusterreceiver.NewFactory(),
			hostmetricsreceiver.NewFactory(),
			httpcheckreceiver.NewFactory(),
			k8sobjectsreceiver.NewFactory(),
			receivercreator.NewFactory(),
			redisreceiver.NewFactory(),
			nginxreceiver.NewFactory(),
			jaegerreceiver.NewFactory(),
			zipkinreceiver.NewFactory(),
			fbreceiver.NewFactory(),
			mbreceiver.NewFactory(),
			jmxreceiver.NewFactory(),
			nopreceiver.NewFactory(),
		}
		// some receivers should only be available when
		// not in fips mode due to restrictions on crypto usage
		receivers = addNonFipsReceivers(receivers)
		factories.Receivers, err = otelcol.MakeFactoryMap(receivers...)
		if err != nil {
			return otelcol.Factories{}, err
		}

		// Processors
		factories.Processors, err = otelcol.MakeFactoryMap[processor.Factory](
			batchprocessor.NewFactory(),
			resourceprocessor.NewFactory(),
			attributesprocessor.NewFactory(),
			cumulativetodeltaprocessor.NewFactory(),
			transformprocessor.NewFactory(),
			filterprocessor.NewFactory(),
			geoipprocessor.NewFactory(),
			k8sattributesprocessor.NewFactory(),
			elasticinframetricsprocessor.NewFactory(),
			resourcedetectionprocessor.NewFactory(),
			memorylimiterprocessor.NewFactory(),
			elastictraceprocessor.NewFactory(),
		)
		if err != nil {
			return otelcol.Factories{}, err
		}

		// Exporters
		exporters := []exporter.Factory{
			otlpexporter.NewFactory(),
			debugexporter.NewFactory(),
			fileexporter.NewFactory(),
			elasticsearchexporter.NewFactory(),
			loadbalancingexporter.NewFactory(),
			otlphttpexporter.NewFactory(),
			nopexporter.NewFactory(),
		}
		// some exporters should only be available when
		// not in fips mode due to restrictions on crypto usage
		exporters = addNonFipsExporters(exporters)
		factories.Exporters, err = otelcol.MakeFactoryMap(exporters...)
		if err != nil {
			return otelcol.Factories{}, err
		}

		factories.Connectors, err = otelcol.MakeFactoryMap[connector.Factory](
			routingconnector.NewFactory(),
			spanmetricsconnector.NewFactory(),
			elasticapmconnector.NewFactory(),
			forwardconnector.NewFactory(),
		)
		if err != nil {
			return otelcol.Factories{}, err
		}

		extensions := []extension.Factory{
			healthcheckv2extension.NewFactory(),
			memorylimiterextension.NewFactory(),
			filestorage.NewFactory(),
			healthcheckextension.NewFactory(),
			bearertokenauthextension.NewFactory(),
			pprofextension.NewFactory(),
			k8sobserver.NewFactory(),
			apikeyauthextension.NewFactory(),
			apmconfigextension.NewFactory(),
		}
		extensions = append(extensions, extensionFactories...)
		factories.Extensions, err = otelcol.MakeFactoryMap[extension.Factory](extensions...)
		if err != nil {
			return otelcol.Factories{}, err
		}

		return factories, err
	}
}

package ext

const (
	// Resource allows datadog resource to be set as a tag e.g. span.SetTag(ext.Resource, "GET /")
	Resource = "datadog.resource"

	// Type sets datadog type as a tag e.g. span.SetTag(ext.Type, datadog.TypeWeb)
	Type = "datadog.type"

	// Service allows the service to be overridden for a specific span.SetTag(ext.Service, "my-service")
	Service = "datadog.service"

	// Error indicates an error has occurred span.SetTag(ext.Error, err)
	Error = "datadog.error"
)

package providers

// Holds a discovered cloud resource endpoint, source tags, and provider-specific metadata.
type Resource struct {
	Host     string                 // Hostname or endpoint address.
	Port     int                    // Endpoint port number.
	Tags     map[string]string      // Source resource tags with their original names and values.
	Metadata map[string]interface{} // Provider-specific additional data.
}

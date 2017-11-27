package bridge

import "fmt"

// ErrInvalidEndpointConfig error is returned when an endpoint create is attempted with an invalid endpoint configuration.
type ErrInvalidEndpointConfig struct{}

func (eiec *ErrInvalidEndpointConfig) Error() string {
	return "trying to create an endpoint with an invalid endpoint configuration"
}

// BadRequest denotes the type of this error
func (eiec *ErrInvalidEndpointConfig) BadRequest() {}

// network id for an existing network is not a known id.
type InvalidNetworkIDError string

func (inie InvalidNetworkIDError) Error() string {
	return fmt.Sprintf("invalid network id %s", string(inie))
}

// NotFound denotes the type of this error
func (inie InvalidNetworkIDError) NotFound() {}

// EndpointNotFoundError is returned when the no endpoint
// with the passed endpoint id is found.
type EndpointNotFoundError string

func (enfe EndpointNotFoundError) Error() string {
	return fmt.Sprintf("endpoint not found: %s", string(enfe))
}

// NotFound denotes the type of this error
func (enfe EndpointNotFoundError) NotFound() {}
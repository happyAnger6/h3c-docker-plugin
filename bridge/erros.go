package bridge

// ErrInvalidEndpointConfig error is returned when an endpoint create is attempted with an invalid endpoint configuration.
type ErrInvalidEndpointConfig struct{}

func (eiec *ErrInvalidEndpointConfig) Error() string {
	return "trying to create an endpoint with an invalid endpoint configuration"
}

// BadRequest denotes the type of this error
func (eiec *ErrInvalidEndpointConfig) BadRequest() {}
package apierrors

import (
	"io"
	"io/ioutil"
	"net/http"
)

// Code is a Mesos HTTP v1 API response status code
type Code int

const (
	// MsgNotLeader is returned by Do calls that are sent to a non leading Mesos master.
	MsgNotLeader = "call sent to a non-leading master"
	// MsgAuth is returned by Do calls that are not successfully authenticated.
	MsgAuth = "call not authenticated"
	// MsgUnsubscribed is returned by Do calls that are sent before a subscription is established.
	MsgUnsubscribed = "no subscription established"
	// MsgVersion is returned by Do calls that are sent to an incompatible API version.
	MsgVersion = "incompatible API version"
	// MsgMalformed is returned by Do calls that are malformed.
	MsgMalformed = "malformed request"
	// MsgMediaType is returned by Do calls that are sent with an unsupported media type.
	MsgMediaType = "unsupported media type"
	// MsgRateLimit is returned by Do calls that are rate limited. This is a temporary condition
	// that should clear.
	MsgRateLimit = "rate limited"
	// MsgUnavailable is returned by Do calls that are sent to a master or agent that's in recovery, or
	// does not yet realize that it's the leader. This is a temporary condition that should clear.
	MsgUnavailable = "mesos server unavailable"
	// MsgNotFound could happen if the master or agent libprocess has not yet set up http routes.
	MsgNotFound = "mesos http endpoint not found"

	CodeNotLeader            = Code(http.StatusTemporaryRedirect)
	CodeNotAuthenticated     = Code(http.StatusUnauthorized)
	CodeUnsubscribed         = Code(http.StatusForbidden)
	CodeIncompatibleVersion  = Code(http.StatusConflict)
	CodeMalformedRequest     = Code(http.StatusBadRequest)
	CodeUnsupportedMediaType = Code(http.StatusNotAcceptable)
	CodeRateLimitExceeded    = Code(http.StatusTooManyRequests)
	CodeMesosUnavailable     = Code(http.StatusServiceUnavailable)
	CodeNotFound             = Code(http.StatusNotFound)

	MaxSizeDetails = 4 * 1024 // MaxSizeDetails limits the length of the details message read from a response body
)

var (
	// ErrorTable maps HTTP response codes to their respective Mesos v1 API error messages.
	ErrorTable = map[Code]string{
		CodeNotLeader:            MsgNotLeader,
		CodeMalformedRequest:     MsgMalformed,
		CodeIncompatibleVersion:  MsgVersion,
		CodeUnsubscribed:         MsgUnsubscribed,
		CodeNotAuthenticated:     MsgAuth,
		CodeUnsupportedMediaType: MsgMediaType,
		CodeNotFound:             MsgNotFound,
		CodeMesosUnavailable:     MsgUnavailable,
		CodeRateLimitExceeded:    MsgRateLimit,
	}
)

// Error captures HTTP v1 API error codes and messages generated by Mesos.
type Error struct {
	code    Code   // code is the HTTP response status code generated by Mesos
	message string // message briefly summarizes the nature of the error, possibly includes details from Mesos
}

// IsError returns true for all HTTP status codes that are not considered informational or successful.
func (code Code) IsError() bool {
	return code >= 300
}

// FromResponse returns an `*Error` for a response containing a status code that indicates an error condition.
// The response body (if any) is captured in the Error.Details field.
// Returns nil for nil responses and responses with non-error status codes.
// See IsErrorCode.
func FromResponse(res *http.Response) error {
	if res == nil {
		return nil
	}

	code := Code(res.StatusCode)
	if !code.IsError() {
		// non-error HTTP response codes don't generate errors
		return nil
	}

	var details string

	if res.Body != nil {
		defer res.Body.Close()
		buf, _ := ioutil.ReadAll(io.LimitReader(res.Body, MaxSizeDetails))
		details = string(buf)
	}

	return code.Error(details)
}

// Error generates an error from the given status code and detail string.
func (code Code) Error(details string) error {
	if !code.IsError() {
		return nil
	}
	err := &Error{
		code:    code,
		message: ErrorTable[code],
	}
	if details != "" {
		err.message = err.message + ": " + details
	}
	return err
}

// Error implements error interface
func (e *Error) Error() string { return e.message }

// Temporary returns true if the error is a temporary condition that should eventually clear.
func (e *Error) Temporary() bool {
	switch e.code {
	// TODO(jdef): NotFound **could** be a temporary error because there's a race at mesos startup in which the
	// HTTP server responds before the internal listeners have been initialized. But it could also be reported
	// because the client is accessing an invalid endpoint; as of right now, a client cannot distinguish between
	// these cases.
	// https://issues.apache.org/jira/browse/MESOS-7697
	case CodeRateLimitExceeded, CodeMesosUnavailable:
		return true
	default:
		return false
	}
}

// CodesIndicatingSubscriptionLoss is a set of apierror.Code entries which each indicate that
// the event subscription stream has been severed between the scheduler and mesos. It's respresented
// as a public map variable so that clients can program additional error codes (if such are discovered)
// without hacking the code of the mesos-go library directly.
var CodesIndicatingSubscriptionLoss = func(codes ...Code) map[Code]struct{} {
	result := make(map[Code]struct{}, len(codes))
	for _, code := range codes {
		result[code] = struct{}{}
	}
	return result
}(
	// expand this list as we discover other errors that guarantee we've lost our event subscription.
	CodeUnsubscribed,
)

// SubscriptionLoss returns true if the error indicates that the event subscription stream has been severed
// between mesos and a mesos client.
func (e *Error) SubscriptionLoss() (result bool) {
	_, result = CodesIndicatingSubscriptionLoss[e.code]
	return
}

// Matches returns true if the given error is an API error with a matching error code
func (code Code) Matches(err error) bool {
	if err == nil {
		return !code.IsError()
	}
	apiErr, ok := err.(*Error)
	return ok && apiErr.code == code
}
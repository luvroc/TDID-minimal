package errors

import "fmt"

type Code string

const (
	CodeInvalidInput    Code = "INVALID_INPUT"
	CodeSessionExpired  Code = "SESSION_EXPIRED"
	CodeInvalidNonce    Code = "INVALID_NONCE"
	CodeNonceReused     Code = "NONCE_REUSED"
	CodeChainInvokeFail Code = "CHAIN_INVOKE_FAILED"
	CodePeerAuthFailed  Code = "PEER_AUTH_FAILED"
	CodeInternal        Code = "INTERNAL"
)

type AppError struct {
	Code    Code
	Message string
	Cause   error
}

func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
}

func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func New(code Code, message string, cause error) error {
	return &AppError{Code: code, Message: message, Cause: cause}
}

package afscp

import (
	"errors"
	"fmt"
)

const ContractVersion = "jvs.afscp.direct.v1"

type Command string

const (
	CommandSave    Command = "save"
	CommandList    Command = "list"
	CommandRestore Command = "restore"
	CommandStatus  Command = "status"
	CommandDoctor  Command = "doctor"
)

type Status string

const (
	StatusAccepted         Status = "accepted"
	StatusRunning          Status = "running"
	StatusSucceeded        Status = "succeeded"
	StatusFailed           Status = "failed"
	StatusRecoveryRequired Status = "recovery_required"
)

type ErrorCode string

const (
	ErrorCodeInvalidArgument         ErrorCode = "JVS_INVALID_ARGUMENT"
	ErrorCodeLocked                  ErrorCode = "JVS_LOCKED"
	ErrorCodeMetadataInvalid         ErrorCode = "JVS_METADATA_INVALID"
	ErrorCodeSavePointNotFound       ErrorCode = "JVS_SAVE_POINT_NOT_FOUND"
	ErrorCodeCloneUnavailable        ErrorCode = "JVS_CLONE_UNAVAILABLE"
	ErrorCodeCloneFailed             ErrorCode = "JVS_CLONE_FAILED"
	ErrorCodeJournalRecoveryRequired ErrorCode = "JVS_JOURNAL_RECOVERY_REQUIRED"
	ErrorCodeInternal                ErrorCode = "JVS_INTERNAL"
)

const (
	ExitSuccess         = 0
	ExitInternal        = 1
	ExitInvalidArgument = 2
	ExitMetadata        = 3
	ExitLocked          = 4
	ExitStorage         = 5
)

type Error struct {
	Code      ErrorCode `json:"code"`
	Message   string    `json:"message"`
	Retryable bool      `json:"retryable"`
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func NewError(code ErrorCode, message string, retryable bool) *Error {
	return &Error{Code: code, Message: message, Retryable: retryable}
}

func ExitCode(err error) int {
	if err == nil {
		return ExitSuccess
	}
	var directErr *Error
	if !errors.As(err, &directErr) {
		return ExitInternal
	}
	switch directErr.Code {
	case ErrorCodeInvalidArgument, ErrorCodeSavePointNotFound:
		return ExitInvalidArgument
	case ErrorCodeMetadataInvalid, ErrorCodeJournalRecoveryRequired:
		return ExitMetadata
	case ErrorCodeLocked:
		return ExitLocked
	case ErrorCodeCloneUnavailable, ErrorCodeCloneFailed:
		return ExitStorage
	default:
		return ExitInternal
	}
}

type Envelope struct {
	Contract string  `json:"contract"`
	Command  Command `json:"command"`
	OK       bool    `json:"ok"`
	Status   Status  `json:"status"`
	Data     any     `json:"data"`
	Error    *Error  `json:"error"`
}

func SuccessEnvelope(command Command, data any) Envelope {
	return Envelope{
		Contract: ContractVersion,
		Command:  command,
		OK:       true,
		Status:   StatusSucceeded,
		Data:     data,
		Error:    nil,
	}
}

func ErrorEnvelope(command Command, err error) Envelope {
	directErr := directError(err)
	return Envelope{
		Contract: ContractVersion,
		Command:  command,
		OK:       false,
		Status:   errorStatus(directErr),
		Data:     nil,
		Error:    directErr,
	}
}

func directError(err error) *Error {
	if err == nil {
		return nil
	}
	var directErr *Error
	if errors.As(err, &directErr) {
		return directErr
	}
	return NewError(ErrorCodeInternal, "unexpected internal failure", false)
}

func errorStatus(err *Error) Status {
	if err == nil {
		return StatusFailed
	}
	if err.Code == ErrorCodeJournalRecoveryRequired {
		return StatusRecoveryRequired
	}
	return StatusFailed
}

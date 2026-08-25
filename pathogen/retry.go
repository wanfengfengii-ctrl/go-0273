package pathogen

import "strawberry-vitro-acclimation-gate/domain"

// Fault outcome error codes for the four scriptable failure modes. They are
// stable so tests can assert the exact device error without touching protocol
// details.
const (
	ErrDeviceRefused      domain.ErrorCode = "DEVICE_REFUSED"
	ErrDeviceDisconnected domain.ErrorCode = "DEVICE_DISCONNECTED"
	ErrDeviceTimeout      domain.ErrorCode = "DEVICE_TIMEOUT"
	ErrDeviceMalformed    domain.ErrorCode = "MALFORMED_RESPONSE"
)

// maxFaultSteps is the number of scripted failure steps before success. The
// fault script is deterministic: step 0 refuses, 1 disconnects, 2 times out,
// 3 returns a malformed payload, and step 4 onward succeeds.
const maxFaultSteps = 4

// stepOutcome returns the deterministic call status and error code for a given
// fault step. Steps below maxFaultSteps fail with the corresponding fault mode;
// steps at or beyond it succeed.
func stepOutcome(step int) (CallStatus, domain.ErrorCode) {
	if step >= maxFaultSteps {
		return CallSucceeded, ""
	}
	switch step {
	case 0:
		return CallFailedRetry, ErrDeviceRefused
	case 1:
		return CallFailedRetry, ErrDeviceDisconnected
	case 2:
		return CallFailedRetry, ErrDeviceTimeout
	case 3:
		return CallFailedRetry, ErrDeviceMalformed
	default:
		return CallSucceeded, ""
	}
}

// backoff returns the logical-time delay before the next retry after a failed
// attempt. It is an exact doubling so retry schedules are fully deterministic.
func backoff(attempts int) domain.LogicalTime {
	if attempts <= 0 {
		return 1
	}
	return domain.LogicalTime(int64(1) << uint(attempts-1))
}

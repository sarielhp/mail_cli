package outlook

import (
	"testing"

	"github.com/sarielhp/mail_cli/backend/backendtest"
)

func TestOutlookClient_MessageAccessorContextCancellation(t *testing.T) {
	backendtest.AssertAccessorHonorsCancellation(t, &client{})
}

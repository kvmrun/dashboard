// Package auth implements user authentication for the dashboard.
//
// Password verification is delegated to the host's PAM stack via cgo —
// the same mechanism used by sshd/su/login — so no separate user database
// is needed. This package is Linux-only (it links libpam).
package auth

/*
#cgo LDFLAGS: -lpam
#include <stdlib.h>
#include <string.h>
#include <security/pam_appl.h>
#include <security/pam_modules.h>

// PAM conversation callback. When the stack asks for the password
// (PROMPT_ECHO_OFF), we answer with a copy of the value posted by the
// HTTP client. PAM frees the response strings after use, so we must
// hand it its own copy (strdup) — never the caller-owned buffer.
static int dash_pam_conv(int num_msg, const struct pam_message **msg,
                         struct pam_response **resp, void *appdata_ptr) {
	*resp = calloc(num_msg, sizeof(struct pam_response));
	if (*resp == NULL) {
		return PAM_PERM_DENIED;
	}
	for (int i = 0; i < num_msg; i++) {
		if (msg[i]->msg_style == PAM_PROMPT_ECHO_OFF || msg[i]->msg_style == PAM_PROMPT_ECHO_ON) {
			(*resp)[i].resp = strdup((const char *)appdata_ptr);
			if ((*resp)[i].resp == NULL) {
				return PAM_PERM_DENIED;
			}
		}
	}
	return PAM_SUCCESS;
}

static int dash_pam_auth(const char *service, const char *username, const char *password) {
	struct pam_conv conv = {dash_pam_conv, (void *)password};
	struct pam_handle *pamh = NULL;
	int ret = pam_start(service, username, &conv, &pamh);
	if (ret != PAM_SUCCESS) {
		return ret;
	}
	ret = pam_authenticate(pamh, PAM_SILENT | PAM_DISALLOW_NULL_AUTHTOK);
	int close_ret = pam_end(pamh, ret);
	if (close_ret != PAM_SUCCESS && ret == PAM_SUCCESS) {
		ret = close_ret;
	}
	return ret;
}

static void dash_pam_wipe(char *s) {
	if (s == NULL) {
		return;
	}
	while (*s) {
		*s++ = 0;
	}
}
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// ErrInvalidCredentials is returned when PAM rejects the password.
var ErrInvalidCredentials = errors.New("invalid username or password")

// PAM authenticates users against a single PAM service name. Safe for
// concurrent use: the password travels through PAM's conversation
// appdata, not global state.
type PAM struct {
	service string
}

// NewPAM returns an authenticator for the given PAM service name. The
// name must refer to an existing /etc/pam.d/<service> file on the host;
// "login" is a safe default on Debian/Ubuntu.
func NewPAM(service string) *PAM {
	return &PAM{service: service}
}

// Authenticate verifies username/password against the PAM stack.
func (p *PAM) Authenticate(username, password string) error {
	cService := C.CString(p.service)
	defer C.free(unsafe.Pointer(cService))
	cUsername := C.CString(username)
	defer C.free(unsafe.Pointer(cUsername))
	cPassword := C.CString(password)
	defer C.free(unsafe.Pointer(cPassword))

	ret := C.dash_pam_auth(cService, cUsername, cPassword)
	C.dash_pam_wipe(cPassword) // scrub the copy that lived in C memory

	if ret == C.PAM_AUTH_ERR {
		return ErrInvalidCredentials
	}
	if ret != C.PAM_SUCCESS {
		return fmt.Errorf("pam: authenticate %q via service %q: status %d", username, p.service, int(ret))
	}
	return nil
}

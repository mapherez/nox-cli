// Package credentials stores one origin-bound secret in the native OS vault.
package credentials

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/zalando/go-keyring"
)

var (
	ErrUnavailable = errors.New("Secure credential storage is unavailable. Unlock your system vault or choose session-only use.")
	ErrNotFound    = errors.New("Enter an API key to continue.")
)

const service = "nox-cli"
const account = "api-key"

type Secret struct {
	Origin string `json:"gatewayOrigin"`
	Key    string `json:"apiKey"`
}

// Vault is deliberately a single slot; implementations must upsert, not append.
type Vault interface {
	Get() (Secret, error)
	Set(Secret) error
	Delete() error
}

type Native struct{ service, account string }

func (n Native) identity() (string, string) {
	if n.service != "" && n.account != "" {
		return n.service, n.account
	}
	return service, account
}

func (n Native) Get() (Secret, error) {
	service, account := n.identity()
	value, err := keyring.Get(service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return Secret{}, ErrNotFound
	}
	if err != nil {
		return Secret{}, ErrUnavailable
	}
	var secret Secret
	if json.Unmarshal([]byte(value), &secret) != nil || ValidateKey(secret.Key) != nil || secret.Origin == "" {
		return Secret{}, ErrUnavailable
	}
	return secret, nil
}

func (n Native) Set(secret Secret) error {
	service, account := n.identity()
	if ValidateKey(secret.Key) != nil || secret.Origin == "" {
		return ErrUnavailable
	}
	value, err := json.Marshal(secret)
	if err != nil || keyring.Set(service, account, string(value)) != nil {
		return ErrUnavailable
	}
	return nil
}

func (n Native) Delete() error {
	service, account := n.identity()
	err := keyring.Delete(service, account)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return ErrUnavailable
	}
	return nil
}

func ValidateKey(value string) error {
	if len(value) != 51 {
		return errors.New("Invalid API key length. Expected 51 characters.")
	}
	if !strings.HasPrefix(value, "noxsync_") {
		return errors.New("Invalid API key format.")
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(value[8:])
	if err != nil || len(data) != 32 {
		return errors.New("Invalid API key format.")
	}
	return nil
}

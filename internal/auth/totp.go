package auth

import (
	"github.com/pquerna/otp/totp"
)

type TOTPSecret struct {
	Secret string
	URL    string
}

func GenerateTOTPSecret(email string) (*TOTPSecret, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Mutane",
		AccountName: email,
	})
	if err != nil {
		return nil, err
	}

	return &TOTPSecret{
		Secret: key.Secret(),
		URL:    key.URL(),
	}, nil
}

func ValidateTOTP(secret, code string) bool {
	return totp.Validate(code, secret)
}

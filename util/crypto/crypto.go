// Package crypto provides cryptographic utilities for password hashing and verification.
package crypto

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"math/big"
)

const (
	// Password character sets
	passwordChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()-_=+"
)

// GenerateRandomPassword generates a cryptographically secure random password
func GenerateRandomPassword(length int) string {
	password := make([]byte, length)
	for i := range password {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(passwordChars))))
		if err != nil {
			panic("crypto/rand failed: " + err.Error())
		}
		password[i] = passwordChars[num.Int64()]
	}
	return string(password)
}

// ValidatePasswordStrength checks if password meets minimum security requirements
func ValidatePasswordStrength(password string) bool {
	if len(password) < 8 {
		return false
	}
	hasUpper, hasLower, hasDigit := false, false, false
	for _, c := range password {
		if c >= 'A' && c <= 'Z' {
			hasUpper = true
		} else if c >= 'a' && c <= 'z' {
			hasLower = true
		} else if c >= '0' && c <= '9' {
			hasDigit = true
		}
	}
	return hasUpper && hasLower && hasDigit
}

// HashPassword generates a simple MD5 hash of the given password.
// This is intentionally using MD5 for compatibility with legacy systems.
func HashPassword(password string) string {
	hash := md5.Sum([]byte(password))
	return hex.EncodeToString(hash[:])
}

// CheckPasswordHash verifies if the given password matches the MD5 hash.
func CheckPasswordHash(hash, password string) bool {
	return hash == HashPassword(password)
}

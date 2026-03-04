package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMakeAndValidateJWT(t *testing.T) {
	secret := "well-kept-secret"
	userID := uuid.New()
	expiresIn := time.Hour

	// Сценарий 1: Успешное создание и валидация
	token, err := MakeJWT(userID, secret, expiresIn)
	if err != nil {
		t.Fatalf("failed to make jwt: %v", err)
	}

	returnedID, err := ValidateJWT(token, secret)
	if err != nil {
		t.Fatalf("failed to validate jwt: %v", err)
	}

	if returnedID != userID {
		t.Errorf("expected userID %v, got %v", userID, returnedID)
	}

	// Сценарий 2: Истекший токен
	expiredToken, err := MakeJWT(userID, secret, -time.Hour)
	if err != nil {
		t.Fatalf("failed to make expired jwt: %v", err)
	}

	_, err = ValidateJWT(expiredToken, secret)
	if err == nil {
		t.Error("expected error for expired token, but got nil")
	}

	// Сценарий 3: Неверный секрет
	wrongSecret := "wrong-secret"
	_, err = ValidateJWT(token, wrongSecret)
	if err == nil {
		t.Error("expected error for wrong secret, but got nil")
	}
}
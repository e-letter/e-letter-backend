package utils

import "golang.org/x/crypto/bcrypt"

const BcryptCostDefault = 12

func HashPassword(p string) (string, error) {
	return HashPasswordWithCost(p, BcryptCostDefault)
}

func HashPasswordWithCost(p string, cost int) (string, error) {
	if cost < 4 {
		cost = 4
	} else if cost > 31 {
		cost = 31
	}
	b, err := bcrypt.GenerateFromPassword([]byte(p), cost)
	return string(b), err
}

func VerifyPassword(hash, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

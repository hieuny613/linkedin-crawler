package models

// Account represents a user account with email and password
type Account struct {
	Email    string
	Password string
}

// TokenResult represents the result of token extraction from an account
type TokenResult struct {
	Account Account
	Token   string
	Error   error
}

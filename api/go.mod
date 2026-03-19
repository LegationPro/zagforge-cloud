module github.com/LegationPro/zagforge-mvp-impl/api

go 1.26.1

require (
	github.com/LegationPro/zagforge-mvp-impl/shared/go v0.0.0
	github.com/jackc/pgx/v5 v5.8.0
	github.com/joho/godotenv v1.5.1
)

require (
	github.com/go-chi/chi/v5 v5.2.5 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)

replace github.com/LegationPro/zagforge-mvp-impl/shared/go => ../shared/go

package yamlx

import (
	"os"
	"testing"
)

// --- Benchmarks ---

func BenchmarkUnmarshalPlain(b *testing.B) {
	yml := []byte(`
name: lighthouse
version: v1.2.3
port:
  - 30303
  - 8545
network: testnet
`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var cfg map[string]any
		Unmarshal(yml, &cfg)
	}
}

func BenchmarkUnmarshalEnvVars(b *testing.B) {
	os.Setenv("BENCH_NAME", "lighthouse")
	os.Setenv("BENCH_PORT", "8080")
	defer os.Unsetenv("BENCH_NAME")
	defer os.Unsetenv("BENCH_PORT")

	yml := []byte(`
name: ${BENCH_NAME}
port: ${BENCH_PORT}
url: http://${BENCH_NAME:-localhost}:${BENCH_PORT:-3000}/api
`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var cfg map[string]any
		Unmarshal(yml, &cfg)
	}
}

func BenchmarkUnmarshalYamlVars(b *testing.B) {
	yml := []byte(`
base_url: http://localhost
port: 8080
endpoint: $base_url:$port/api
health: $base_url:$port/health
version: v1
`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var cfg map[string]any
		Unmarshal(yml, &cfg)
	}
}

func BenchmarkUnmarshalConditionals(b *testing.B) {
	os.Setenv("BENCH_ENV", "production")
	defer os.Unsetenv("BENCH_ENV")

	yml := []byte(`
env: production
port: !if "$env" == "production" 443 else 8080
log: !if "${BENCH_ENV}" == "production" warn else debug
region: !if "$env" != "dev" us-east-1 us-west-2
`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var cfg map[string]any
		Unmarshal(yml, &cfg)
	}
}

func BenchmarkUnmarshalEnumValidation(b *testing.B) {
	os.Setenv("BENCH_ENV", "production")
	defer os.Unsetenv("BENCH_ENV")

	yml := []byte(`
env: ${BENCH_ENV:|production|staging|development}
`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var cfg map[string]any
		Unmarshal(yml, &cfg)
	}
}

func BenchmarkUnmarshalInclude(b *testing.B) {
	tmpDir := b.TempDir()
	writeFile := func(name, content string) {
		os.WriteFile(tmpDir+"/"+name, []byte(content), 0644)
	}
	writeFile("base.yaml", "type: p2p\nsubnet: 10.0.0.0/24\n")
	writeFile("ports.yaml", "- 30303\n- 8545\n")

	yml := []byte(`
name: lighthouse
network: !include base.yaml
ports: !include ports.yaml
`)

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var cfg map[string]any
		Unmarshal(yml, &cfg)
	}
}

func BenchmarkUnmarshalAllFeatures(b *testing.B) {
	tmpDir := b.TempDir()
	writeFile := func(name, content string) {
		os.WriteFile(tmpDir+"/"+name, []byte(content), 0644)
	}
	writeFile("network.yaml", "type: p2p\nsubnet: 10.0.0.0/24\n")

	os.Setenv("BENCH_DB", "postgres://localhost:5432/db")
	defer os.Unsetenv("BENCH_DB")

	yml := []byte(`
name: ${APP_NAME:-myapp}
env: production
region: us-east-1
port: !if "$env" == "production" 443 else 8080
log: !if "$env" == "production" warn else debug
network: !include network.yaml
db: ${BENCH_DB:?}
mode: ${APP_ENV:|production|staging|development}
`)

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var cfg map[string]any
		Unmarshal(yml, &cfg)
	}
}

func BenchmarkUnmarshalLargeYAML(b *testing.B) {
	yml := []byte(`
services:
  web:
    image: nginx:latest
    ports:
      - "80:80"
      - "443:443"
    environment:
      - NODE_ENV=production
      - DATABASE_URL=postgres://db:5432/app
      - REDIS_URL=redis://cache:6379
      - AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
      - AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
      - ./ssl:/etc/nginx/ssl:ro
      - static-files:/usr/share/nginx/html
    networks:
      - frontend
      - backend
    depends_on:
      - api
      - cache
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost/health"]
      interval: 30s
      timeout: 10s
      retries: 3
  api:
    image: myapp:latest
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=postgres://db:5432/app
      - REDIS_URL=redis://cache:6379
      - JWT_SECRET=mysecretkey
      - LOG_LEVEL=info
    networks:
      - backend
    depends_on:
      - db
      - cache
    restart: unless-stopped
  db:
    image: postgres:15
    environment:
      - POSTGRES_DB=app
      - POSTGRES_USER=admin
      - POSTGRES_PASSWORD=secret
    volumes:
      - pgdata:/var/lib/postgresql/data
    networks:
      - backend
    restart: unless-stopped
  cache:
    image: redis:7-alpine
    networks:
      - backend
    restart: unless-stopped
volumes:
  pgdata:
  static-files:
networks:
  frontend:
  backend:
`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var cfg map[string]any
		Unmarshal(yml, &cfg)
	}
}

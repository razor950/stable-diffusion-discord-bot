FROM golang:1.21-alpine3.19 AS build

# Set destination for COPY
WORKDIR /usr/src/app

# Download Go modules
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o stable_diffusion_bot

FROM alpine:3.19 AS final

WORKDIR /app

COPY --from=build /usr/src/app/stable_diffusion_bot /app/stable_diffusion_bot

ENTRYPOINT ["/app/stable_diffusion_bot"]
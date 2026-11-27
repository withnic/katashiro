# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.27.4@sha256:205c7636d74e0ffbe3d007162df7d74f859f5a8c8322dfb9f907977686c126cf AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go test ./... \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-w -s" -o /out/katashiro ./cmd/katashiro \
 && go run ./tools/notices > /out/THIRD_PARTY_NOTICES

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian13:nonroot@sha256:8ce6dd87e9823c5b48dcfb2c8604714ccb351ce59607327bcc4957448da279b5
COPY --from=build /out/katashiro /usr/local/bin/katashiro
COPY LICENSE /usr/share/doc/katashiro/LICENSE
COPY --from=build /out/THIRD_PARTY_NOTICES /usr/share/doc/katashiro/THIRD_PARTY_NOTICES
ENV KATASHIRO_STATE_DIR=/home/nonroot/.local/state/katashiro
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/katashiro"]
CMD ["serve", "--port=8080", "--in-memory"]

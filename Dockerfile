FROM public.ecr.aws/docker/library/golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/ecs-benchmark ./cmd/ecs-benchmark

FROM public.ecr.aws/docker/library/alpine:3.22

RUN adduser -D -H -u 10001 appuser
USER appuser

ENV ADDR=:8080
EXPOSE 8080

COPY --from=build /out/ecs-benchmark /ecs-benchmark
ENTRYPOINT ["/ecs-benchmark"]

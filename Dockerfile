FROM reg.mindwork.cool/golang:1.27-alpine AS builder
WORKDIR /app
COPY . /app
RUN go build -o ./bin/app .

FROM reg.mindwork.cool/alpine
WORKDIR /app
COPY --from=builder /app/bin/app /app
COPY --from=builder /app/db /app/db
COPY --from=builder /app/docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh && mkdir -p /app/data

ENV AIRWAY_ENV=production
ENV LISTEN=:1905
ENV TZ="Asia/Shanghai"

EXPOSE 1905

ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["server"]

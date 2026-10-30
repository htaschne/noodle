FROM maven:3.9.11-eclipse-temurin-25-alpine AS builder

WORKDIR /app

COPY pom.xml ./
COPY src ./src
RUN --mount=type=cache,target=/root/.m2 mvn --batch-mode package

FROM eclipse-temurin:25-jre-alpine

WORKDIR /app

RUN addgroup -S noodle \
    && adduser -S noodle -G noodle \
    && mkdir -p /data \
    && chown noodle:noodle /data

COPY --from=builder /app/target/notifications-0.0.1-SNAPSHOT.jar app.jar

USER noodle

ENTRYPOINT ["java", "--enable-native-access=ALL-UNNAMED", "-jar", "/app/app.jar"]

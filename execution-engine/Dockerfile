FROM eclipse-temurin:21-jre
WORKDIR /app
COPY target/loomspan-cloud-execution-engine-0.1.0-SNAPSHOT.jar execution-engine.jar
USER 10001:10001
ENTRYPOINT ["java", "-jar", "/app/execution-engine.jar"]

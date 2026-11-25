FROM eclipse-temurin:21-jre
WORKDIR /app
COPY engine.jar /app/engine.jar
COPY fixture/ /app/fixture/
ENTRYPOINT ["java", "-Dloader.path=/app/fixture/", "-cp", "/app/engine.jar", "org.springframework.boot.loader.launch.PropertiesLauncher", "--spring.main.sources=ai.loomspan.cloud.engine.fixture.MilestoneExecutorConfiguration"]

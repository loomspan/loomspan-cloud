package ai.loomspan.cloud.engine;

import ai.loomspan.cloud.engine.fixture.MilestoneExecutorConfiguration;
import org.springframework.boot.builder.SpringApplicationBuilder;
import org.springframework.context.ConfigurableApplicationContext;
import tools.jackson.databind.json.JsonMapper;

import java.io.ByteArrayInputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.SequenceInputStream;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.Map;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;

import static org.assertj.core.api.Assertions.assertThat;

final class InvocationTestEngine implements AutoCloseable {
    static final JsonMapper JSON = JsonMapper.builder().build();
    static final String NAMESPACE = "milestone";
    static final String SKILL = "echo-release";
    static final String CALLER = """
            {"issuer":"loomspan-cloud","subject":"test-user","roles":[]}
            """;
    private static final String BOUNDARY = "loomspan-invocation-test";
    final ConfigurableApplicationContext context;
    final HttpClient client;
    private final String host;

    InvocationTestEngine(EngineClientIdentityTest.AuthorizedFixture tls, Class<?>... sources) throws Exception {
        context = new SpringApplicationBuilder(ExecutionEngineApplication.class, MilestoneExecutorConfiguration.class)
                .sources(sources)
                .profiles("k3s", "milestone-fixture")
                .run(tls.launchArguments());
        host = "https://127.0.0.1:" + PublicationControllerTest.port(context);
        client = tls.openClient();
    }

    static String leaf(String release) {
        return """
                name: echo-release
                description: Echo the publication label and input
                executor: milestone-host:echo-release
                input_schema:
                  type: object
                  required: [text]
                  properties:
                    text: {type: string}
                config: {release: %s}
                """.formatted(release);
    }

    void publish(String version, String configuration, Map<String, String> skills) throws Exception {
        var metadata = JSON.createObjectNode();
        metadata.put("namespace", NAMESPACE);
        metadata.put("publicationVersion", version);
        metadata.putObject("variableValues");
        metadata.set("runtimeSettings", JSON.readTree(
                PublicationControllerTest.RUNTIME_SETTINGS.replace("ONERROR", "NEVER")));
        var body = HttpRequest.BodyPublishers.concat(
                HttpRequest.BodyPublishers.ofString("--" + BOUNDARY + "\r\n"
                        + "Content-Disposition: form-data; name=\"metadata\"\r\n"
                        + "Content-Type: application/json\r\n\r\n" + metadata + "\r\n--" + BOUNDARY + "\r\n"
                        + "Content-Disposition: form-data; name=\"pack\"; filename=\"publication.zip\"\r\n"
                        + "Content-Type: application/zip\r\n\r\n"),
                HttpRequest.BodyPublishers.ofByteArray(PublicationControllerTest.pack(NAMESPACE, configuration, skills)),
                HttpRequest.BodyPublishers.ofString("\r\n--" + BOUNDARY + "--\r\n"));
        var response = client.send(request("publish", "multipart/form-data; boundary=" + BOUNDARY, body),
                HttpResponse.BodyHandlers.ofString());
        assertThat(response.statusCode()).as(response.body()).isEqualTo(204);
    }

    void publishLeaf(String release) throws Exception {
        publish(release, "loomspan: {}", Map.of("loomspan/skills/echo-release.yaml", leaf(release)));
    }

    static String invocation(String skill, String input, String caller, boolean view) {
        return "{\"namespace\":\"milestone\",\"skill\":\"" + skill + "\",\"input\":" + input
                + ",\"caller\":" + caller + (view ? ",\"executionView\":\"always\"" : "") + "}";
    }

    HttpResponse<String> invoke(String body) throws Exception {
        return client.send(invokeRequest(HttpRequest.BodyPublishers.ofString(body)), HttpResponse.BodyHandlers.ofString());
    }

    HttpRequest invokeRequest(HttpRequest.BodyPublisher body) {
        return request("invoke", "application/json", body);
    }

    InTransit sendInTransit(String body) throws InterruptedException {
        byte[] first = body.substring(0, body.length() - 1).getBytes(StandardCharsets.UTF_8);
        byte[] last = body.substring(body.length() - 1).getBytes(StandardCharsets.UTF_8);
        var readingFinalByte = new CountDownLatch(1);
        var release = new CountDownLatch(1);
        var publisher = HttpRequest.BodyPublishers.ofInputStream(() -> new SequenceInputStream(
                new ByteArrayInputStream(first), new InputStream() {
                    private final InputStream delegate = new ByteArrayInputStream(last);

                    @Override
                    public int read() throws IOException {
                        readingFinalByte.countDown();
                        try {
                            if (!release.await(20, TimeUnit.SECONDS)) {
                                throw new IOException("Held request timed out");
                            }
                        } catch (InterruptedException failure) {
                            Thread.currentThread().interrupt();
                            throw new IOException(failure);
                        }
                        return delegate.read();
                    }
                }));
        var response = client.sendAsync(invokeRequest(publisher), HttpResponse.BodyHandlers.ofString());
        var transit = new InTransit(response, release);
        if (!readingFinalByte.await(10, TimeUnit.SECONDS)) {
            transit.close();
            throw new AssertionError("The HTTP client did not start sending the incomplete request");
        }
        return transit;
    }

    record InTransit(CompletableFuture<HttpResponse<String>> response, CountDownLatch release) implements AutoCloseable {
        HttpResponse<String> deliver() throws Exception {
            release.countDown();
            return response.get(20, TimeUnit.SECONDS);
        }

        @Override
        public void close() {
            release.countDown();
            response.cancel(true);
        }
    }

    private HttpRequest request(String path, String mediaType, HttpRequest.BodyPublisher body) {
        return HttpRequest.newBuilder(URI.create(host + "/internal/v1/" + path))
                .timeout(Duration.ofSeconds(30))
                .header("Content-Type", mediaType)
                .POST(body)
                .build();
    }

    @Override
    public void close() {
        client.close();
        context.close();
    }
}

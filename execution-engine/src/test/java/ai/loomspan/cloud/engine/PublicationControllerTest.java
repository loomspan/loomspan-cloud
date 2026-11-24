package ai.loomspan.cloud.engine;

import ai.loomspan.api.ExecutionConfiguration;
import ai.loomspan.api.NamespaceVisibility;
import ai.loomspan.api.PreparedSkillUpdate;
import ai.loomspan.api.SkillCatalog;
import ai.loomspan.api.SkillDocument;
import ai.loomspan.api.SkillKind;
import ai.loomspan.api.SkillReloadException;
import ai.loomspan.api.SkillValidationIssue;
import ai.loomspan.api.SkillValidationResult;
import ai.loomspan.api.ValidatedSkill;
import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.TestInstance;
import org.springframework.boot.autoconfigure.EnableAutoConfiguration;
import org.springframework.boot.builder.SpringApplicationBuilder;
import org.springframework.boot.test.context.TestConfiguration;
import org.springframework.boot.web.server.servlet.context.ServletWebServerApplicationContext;
import org.springframework.context.ConfigurableApplicationContext;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Import;
import tools.jackson.databind.json.JsonMapper;
import tools.jackson.databind.node.ObjectNode;

import java.io.ByteArrayOutputStream;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.Collection;
import java.util.List;
import java.util.Map;
import java.util.zip.ZipEntry;
import java.util.zip.ZipOutputStream;

import static org.assertj.core.api.Assertions.assertThat;

@TestInstance(TestInstance.Lifecycle.PER_CLASS)
class PublicationControllerTest {
    static final String RUNTIME_SETTINGS = """
            {
              "session": {
                "mission-timeout": "60s",
                "max-depth": 32,
                "attachments": {"max-size": "20971520B"},
                "quotas": {
                  "max-skill-invocations": 64,
                  "max-tool-invocations": 128,
                  "max-linter-retries": 32,
                  "max-model-calls": 64,
                  "max-provider-attempts": 192,
                  "max-usage-units": 200000
                }
              },
              "execution-trace": {"persistence": "ONERROR"}
            }
            """;
    private static final String BOUNDARY = "loomspan-publication-http-test";
    private static final JsonMapper JSON = JsonMapper.builder().build();
    private static final List<String> RUNTIME_FIELDS = List.of("session.mission-timeout", "session.max-depth",
            "session.attachments.max-size", "session.quotas.max-skill-invocations", "session.quotas.max-tool-invocations",
            "session.quotas.max-linter-retries", "session.quotas.max-model-calls", "session.quotas.max-provider-attempts",
            "session.quotas.max-usage-units", "execution-trace.persistence");

    private ConfigurableApplicationContext context;
    private HttpClient client;
    private RecordingReloader reloader;
    private byte[] portable;

    @BeforeAll
    void start() throws Exception {
        context = new SpringApplicationBuilder(WebApp.class)
                .properties("server.address=127.0.0.1", "server.port=0", "spring.main.banner-mode=off").run();
        client = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(5)).build();
        reloader = context.getBean(RecordingReloader.class);
        portable = pack("milestone", "loomspan: {}\n", Map.of());
    }

    @AfterAll
    void stop() {
        if (client != null) {
            client.close();
        }
        if (context != null) {
            context.close();
        }
    }

    @BeforeEach
    void resetReloader() {
        reloader.validation = new SkillValidationResult(List.of(), List.of());
        reloader.validatedConfiguration = null;
        reloader.validationFailure = null;
        reloader.publishFailure = null;
        reloader.prepared = null;
        reloader.published = null;
    }

    @Test
    void validatesPortablePublicationWithCompleteRuntimeSettings() throws Exception {
        var response = request("validate", metadata("milestone"));
        assertThat(response.statusCode()).isEqualTo(200);
        assertThat(response.body()).isEqualTo("{\"valid\":true,\"issues\":[],\"skills\":[]}");
        assertThat(reloader.validatedConfiguration).isNotNull();
    }

    @Test
    void requiresEveryRuntimeFieldAndItsWireTypeOnBothEndpoints() throws Exception {
        for (String operation : List.of("validate", "publish")) {
            for (String field : RUNTIME_FIELDS) {
                String path = "runtimeSettings." + field;
                String member = member(path);
                var absent = requestMetadata(operation);
                parent(absent, path).remove(member);
                assertFailure(request(operation, absent.toString()), 400, path);

                var wrongType = requestMetadata(operation);
                ObjectNode parent = parent(wrongType, path);
                if (parent.get(member).isIntegralNumber()) {
                    parent.put(member, "32");
                } else {
                    parent.put(member, 32);
                }
                assertFailure(request(operation, wrongType.toString()), 400, path);
            }
            for (String path : List.of("runtimeSettings", "runtimeSettings.session", "runtimeSettings.session.attachments",
                    "runtimeSettings.session.quotas", "runtimeSettings.execution-trace", "namespace")) {
                var nullField = requestMetadata(operation);
                parent(nullField, path).putNull(member(path));
                assertFailure(request(operation, nullField.toString()), 400, path);
            }
        }
        assertNoReloaderCalls();
    }

    @Test
    void rejectsInvalidSettingValuesWithoutCoercion() throws Exception {
        var invalidValues = Map.of(
                "runtimeSettings.session.max-depth", List.of("0", "32.0", "2147483648", "null", "-1"),
                "runtimeSettings.session.mission-timeout", List.of("\"none\"", "\"-1s\"", "\"0s\"", "\"60\""),
                "runtimeSettings.session.attachments.max-size", List.of("\"20971520\"", "\"-1B\"", "\"10XX\""),
                "runtimeSettings.session.quotas.max-usage-units", List.of("-1"),
                "runtimeSettings.execution-trace.persistence", List.of("\"onerror\""),
                "namespace", List.of("\"bad_namespace\""));
        for (String operation : List.of("validate", "publish")) {
            for (var field : invalidValues.entrySet()) {
                for (String value : field.getValue()) {
                    assertFailure(request(operation, withValue(operation, field.getKey(), value)), 400, field.getKey());
                }
            }
        }
        assertNoReloaderCalls();
    }

    @Test
    void rejectsMalformedMultipartAndMetadataAsPlainText() throws Exception {
        for (String operation : List.of("validate", "publish")) {
            String metadata = requestMetadata(operation).toString();
            var metadataPart = part("metadata", "application/json", metadata.getBytes(StandardCharsets.UTF_8));
            var packPart = part("pack", "application/zip", portable);
            assertFailure(send(operation, List.of(packPart)), 400, "metadata");
            assertFailure(send(operation, List.of(metadataPart)), 400, "pack");
            assertFailure(send(operation, List.of(packPart, metadataPart, packPart)), 400, "Duplicate multipart part named pack");
            assertFailure(send(operation, List.of(packPart, metadataPart, metadataPart)), 400, "Duplicate multipart part named metadata");
            assertFailure(send(operation, List.of(packPart, part("metadata", "text/plain", new byte[0]))), 400, "application/json");
            assertFailure(send(operation, List.of(metadataPart, part("pack", "text/plain", portable))), 400, "application/zip");
            assertFailure(send(operation, List.of(metadataPart, packPart, part("unknown", "text/plain", new byte[0]))),
                    400, "only the pack and metadata");
            assertFailure(request(operation, "{"), 400, "end-of-input");
            assertFailure(request(operation, "{\"namespace\":\"milestone\"," + metadata.substring(1)), 400, "Duplicate Object property");
            assertFailure(request(operation, metadata + " {}"), 400, "Trailing token");
            assertFailure(send(operation, List.of(metadataPart, part("pack", "application/zip",
                    new byte[PublicationZipAdapter.MAX_ZIP_BYTES + 1]))), 413, "upload size");
            assertFailure(sendRequest(operation, "application/json", HttpRequest.BodyPublishers.ofString(metadata)),
                    415, "application/json");
        }
        assertNoReloaderCalls();
    }

    @Test
    void checksPublicationMetadataAndAcknowledgesOnlyAfterPublish() throws Exception {
        var metadata = requestMetadata("publish");
        for (String absent : List.of("publicationVersion", "variableValues")) {
            var changed = metadata.deepCopy();
            changed.remove(absent);
            assertFailure(request("publish", changed.toString()), 400, absent);
        }
        assertFailure(request("publish", withValue("publish", "publicationVersion", "\"  \"")), 400, "publicationVersion");
        for (String map : List.of("null", "[]")) {
            assertFailure(request("publish", withValue("publish", "variableValues", map)), 400, "variableValues");
        }
        for (String value : List.of("null", "42", "\"   \"")) {
            assertFailure(request("publish", withValue("publish", "variableValues", "{\"app.key\":" + value + "}")),
                    400, "variableValues.app.key");
        }
        assertNoReloaderCalls();
        var response = request("publish", metadata.toString());
        assertThat(response.statusCode()).as(response.body()).isEqualTo(204);
        assertThat(response.body()).isEmpty();
        assertThat(reloader.published).isSameAs(reloader.prepared);
        assertThat(reloader.prepared.closed).isTrue();
    }

    @Test
    void appliesSuppliedSettingsAndKeepsTraceDirectoryHostOwned() throws Exception {
        for (String policy : List.of("NEVER", "ONERROR", "ALWAYS")) {
            var metadata = requestMetadata("validate");
            var trace = parent(metadata, "runtimeSettings.execution-trace.persistence");
            trace.put("persistence", policy);
            trace.put("directory", "tenant-trace-directory");
            parent(metadata, "runtimeSettings.session.max-depth").put("max-depth", 12);
            var response = request("validate", metadata.toString());
            assertThat(response.statusCode()).as(response.body()).isEqualTo(200);
            assertThat(reloader.validatedConfiguration.yaml()).contains("max-depth: 12", policy)
                    .doesNotContain("tenant-trace-directory");
        }
    }

    @Test
    void reportsWarningsAndSkillsAndPreservesInterruptedAndPublishFailureMessages() throws Exception {
        reloader.validation = new SkillValidationResult(List.of(new SkillValidationIssue(SkillValidationIssue.Severity.WARNING,
                        "loomspan/skills/nested/probe.yaml", null, null, "A warning")),
                        List.of(new ValidatedSkill("milestone", "probe", SkillKind.YAML)));
        var response = request("validate", metadata("milestone"));
        assertThat(response.statusCode()).isEqualTo(200);
        assertThat(JSON.readTree(response.body())).isEqualTo(JSON.readTree("""
                {"valid":true,"issues":[{"severity":"WARNING","sourceName":"loomspan/skills/nested/probe.yaml",
                  "message":"A warning"}],"skills":[{"namespace":"milestone","localName":"probe","kind":"YAML"}]}
                """));

        reloader.validationFailure = new SkillReloadException("Validation was interrupted — please retry");
        response = request("validate", metadata("milestone"));
        assertFailure(response, 400, "Validation was interrupted — please retry");
        assertThat(response.body()).isEqualTo("Validation was interrupted — please retry");

        reloader.publishFailure = new SkillReloadException("Base generation changed");
        var failedPublish = request("publish", requestMetadata("publish").toString());
        assertFailure(failedPublish, 400, "Base generation changed");
        assertThat(failedPublish.body()).isEqualTo("Base generation changed");
        assertThat(reloader.prepared.closed).isTrue();
    }

    @Test
    void rejectsIneffectiveCredentialsBeforePreparingEveryInstall() throws Exception {
        String yaml = """
                loomspan:
                  connections:
                    tenant:
                      driver: GEMINI
                      gemini: {vertex-ai: true, project-id: tenant, location: us-central1}
                """;
        var metadata = requestMetadata("publish");
        var invalid = pack("milestone", yaml, Map.of());
        for (String version : List.of("same-version", "restored-version", "same-version")) {
            metadata.put("publicationVersion", version);
            assertFailure(send("publish", parts(metadata.toString(), invalid)), 400,
                    "loomspan.connections.tenant.gemini.credentials-json");
        }
        assertNoReloaderCalls();
    }

    private void assertNoReloaderCalls() {
        assertThat(reloader.validatedConfiguration).isNull();
        assertThat(reloader.prepared).isNull();
        assertThat(reloader.published).isNull();
    }

    private static ObjectNode requestMetadata(String operation) {
        var metadata = (ObjectNode) JSON.readTree(metadata("milestone"));
        if (operation.equals("publish")) {
            metadata.put("publicationVersion", "release / @ 1");
            metadata.putObject("variableValues");
        }
        return metadata;
    }

    private static String member(String path) {
        return path.substring(path.lastIndexOf('.') + 1);
    }

    private static ObjectNode parent(ObjectNode object, String path) {
        String[] fields = path.split("\\.");
        for (int index = 0; index < fields.length - 1; index++) {
            object = (ObjectNode) object.get(fields[index]);
        }
        return object;
    }

    private static String withValue(String operation, String path, String value) {
        var metadata = requestMetadata(operation);
        parent(metadata, path).set(member(path), JSON.readTree(value));
        return metadata.toString();
    }

    private static void assertFailure(HttpResponse<String> response, int status, String diagnostic) {
        assertThat(response.statusCode()).as(response.body()).isEqualTo(status);
        assertThat(response.headers().firstValue("Content-Type").orElseThrow()).startsWith("text/plain");
        assertThat(response.body()).containsIgnoringCase(diagnostic);
    }

    static String metadata(String namespace) {
        return "{\"namespace\":\"" + namespace + "\",\"runtimeSettings\":" + RUNTIME_SETTINGS + "}";
    }

    static int port(ConfigurableApplicationContext context) {
        return ((ServletWebServerApplicationContext) context).getWebServer().getPort();
    }

    static byte[] pack(String namespace, String yaml, Map<String, String> skills) throws Exception {
        var bytes = new ByteArrayOutputStream();
        try (var zip = new ZipOutputStream(bytes)) {
            zipEntry(zip, namespace + "/loomspan/execution.yaml", yaml);
            for (var skill : skills.entrySet()) {
                zipEntry(zip, namespace + "/" + skill.getKey(), skill.getValue());
            }
        }
        return bytes.toByteArray();
    }

    private static void zipEntry(ZipOutputStream zip, String path, String yaml) throws Exception {
        zip.putNextEntry(new ZipEntry(path));
        zip.write(yaml.getBytes(StandardCharsets.UTF_8));
        zip.closeEntry();
    }

    private HttpResponse<String> request(String endpoint, String metadata) throws Exception {
        return send(endpoint, parts(metadata, portable));
    }

    private static List<HttpRequest.BodyPublisher> parts(String metadata, byte[] pack) {
        return List.of(part("metadata", "application/json", metadata.getBytes(StandardCharsets.UTF_8)),
                part("pack", "application/zip", pack));
    }

    private static HttpRequest.BodyPublisher part(String name, String mediaType, byte[] contents) {
        String filename = "pack".equals(name) ? "; filename=\"publication.zip\"" : "";
        return HttpRequest.BodyPublishers.concat(HttpRequest.BodyPublishers.ofString("--" + BOUNDARY + "\r\n"
                        + "Content-Disposition: form-data; name=\"" + name + "\"" + filename + "\r\n"
                        + "Content-Type: " + mediaType + "\r\n\r\n"),
                HttpRequest.BodyPublishers.ofByteArray(contents), HttpRequest.BodyPublishers.ofString("\r\n"));
    }

    private HttpResponse<String> send(String endpoint, List<HttpRequest.BodyPublisher> parts) throws Exception {
        var body = HttpRequest.BodyPublishers.concat(
                HttpRequest.BodyPublishers.concat(parts.toArray(HttpRequest.BodyPublisher[]::new)),
                HttpRequest.BodyPublishers.ofString("--" + BOUNDARY + "--\r\n"));
        return sendRequest(endpoint, "multipart/form-data; boundary=" + BOUNDARY, body);
    }

    private HttpResponse<String> sendRequest(String endpoint, String mediaType, HttpRequest.BodyPublisher body) throws Exception {
        var request = HttpRequest.newBuilder(URI.create("http://127.0.0.1:" + port(context) + "/internal/v1/" + endpoint))
                .timeout(Duration.ofSeconds(10))
                .header("Content-Type", mediaType)
                .POST(body)
                .build();
        return client.send(request, HttpResponse.BodyHandlers.ofString());
    }

    @TestConfiguration(proxyBeanMethods = false)
    @EnableAutoConfiguration(excludeName = {
            "ai.loomspan.autoconfigure.LoomspanJacksonAutoConfiguration",
            "ai.loomspan.autoconfigure.LoomspanAutoConfiguration",
            "ai.loomspan.autoconfigure.LoomspanAiAutoConfiguration",
            "ai.loomspan.autoconfigure.LoomspanObservabilityWebAutoConfiguration"
    })
    @Import({PublicationController.class, PublicationService.class, TenantVertexPolicy.class,
            PublicationZipAdapter.class, PublicationExceptionResolver.class})
    static class WebApp {
        @Bean
        RecordingReloader reloader() {
            return new RecordingReloader();
        }
    }

    static final class RecordingReloader extends PublicationReloaderStub {
        volatile SkillValidationResult validation;
        volatile ExecutionConfiguration validatedConfiguration;
        volatile SkillReloadException validationFailure;
        volatile SkillReloadException publishFailure;
        volatile FixturePreparedUpdate prepared;
        volatile PreparedSkillUpdate published;

        @Override
        public SkillValidationResult validate(String namespace, Collection<SkillDocument> documents,
                                              ExecutionConfiguration configuration, NamespaceVisibility visibility) {
            validatedConfiguration = configuration;
            assertThat(namespace).isEqualTo("milestone");
            assertThat(documents).isEmpty();
            assertThat(visibility).isEqualTo(NamespaceVisibility.PRIVATE);
            if (validationFailure != null) {
                throw validationFailure;
            }
            return validation;
        }

        @Override
        public PreparedSkillUpdate prepare(String namespace, String version, Collection<SkillDocument> documents,
                                           ExecutionConfiguration configuration, NamespaceVisibility visibility,
                                           Map<String, String> variableValues) {
            prepared = new FixturePreparedUpdate();
            assertThat(namespace).isEqualTo("milestone");
            assertThat(version).isEqualTo("release / @ 1");
            assertThat(documents).isEmpty();
            assertThat(visibility).isEqualTo(NamespaceVisibility.PRIVATE);
            assertThat(variableValues).isEmpty();
            return prepared;
        }

        @Override
        public void publish(PreparedSkillUpdate update) {
            assertThat(update).isSameAs(prepared);
            if (publishFailure != null) {
                throw publishFailure;
            }
            published = update;
        }
    }

    private static final class FixturePreparedUpdate implements PreparedSkillUpdate {
        volatile boolean closed;

        @Override
        public String generationId() {
            return "fixture";
        }

        @Override
        public SkillCatalog snapshot() {
            throw new UnsupportedOperationException();
        }

        @Override
        public void close() {
            closed = true;
        }
    }
}

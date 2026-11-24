package ai.loomspan.cloud.engine;

import ai.loomspan.api.ExecutionConfiguration;
import ai.loomspan.api.NamespaceVisibility;
import ai.loomspan.api.PreparedSkillUpdate;
import ai.loomspan.api.SkillDocument;
import ai.loomspan.api.SkillExecutor;
import ai.loomspan.api.SkillReloadException;
import ai.loomspan.api.SkillReloader;
import ai.loomspan.api.SkillTemplate;
import ai.loomspan.api.SkillValidationIssue;
import ai.loomspan.api.extensions.v1.SkillExecutorInvocation;
import ai.loomspan.autoconfigure.LoomspanAiAutoConfiguration;
import ai.loomspan.autoconfigure.LoomspanAutoConfiguration;
import ai.loomspan.autoconfigure.LoomspanJacksonAutoConfiguration;
import ai.loomspan.internal.serialization.AuthoredYamlVariables;
import ai.loomspan.internal.skill.SkillGenerationManager;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;
import org.springframework.boot.autoconfigure.AutoConfigurations;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.boot.test.context.ConfigDataApplicationContextInitializer;
import org.springframework.boot.test.context.TestConfiguration;
import org.springframework.boot.test.context.runner.ApplicationContextRunner;
import org.springframework.context.annotation.Bean;
import org.springframework.mock.web.MockPart;
import tools.jackson.databind.json.JsonMapper;
import tools.jackson.databind.node.ObjectNode;

import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Duration;
import java.util.Collection;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class PublicationGenerationTest {
    private static final JsonMapper JSON = JsonMapper.builder().build();
    private static final String VERSION = "release / @ 1";
    private static final String WORKED_SOURCE = "loomspan/skills/records/read-record.yaml";
    private static final String COLLISION_SOURCE = "loomspan/skills/records/collision.yaml";

    // Framework's loomspan-variable-fixtures/worked, with the owner selected by the test.
    private static final String WORKED_EXECUTION = """
            loomspan:
              connections:
                primary:
                  driver: openai
                  api-key: { $var: provider.key }
                  headers:
                    X-Region: { $var: app.region }
              executor-config:
                OWNER:
                  targets:
                    records:
                      base-url: { $var: records.url }
                      headers:
                        X-Region: { $var: app.region }
                        Accept: application/json
                      regions:
                        - values:
                            - { $var: app.region }
            """;
    private static final String WORKED_SKILL = """
            name: read-record
            description: Read a record from the configured service
            executor: fixture-host:http
            config:
              target: records
              method: GET
              path: { $var: records.lookup-path }
            """;

    @Test
    void validationKeepsServingAndCollectsIndependentAuthoredAndSkillIssues(@TempDir Path directory) throws Exception {
        engine(directory).run(context -> {
            assertThat(context).hasNotFailed();
            var reloader = context.getBean(SkillReloader.class);
            var service = withHostExecutor(reloader);
            var serving = workedPack("sales");
            service.publish(serving, metadata("sales", VERSION, values("sales-key", "west", "/original")));
            var previous = reloader.snapshot("sales");
            var template = context.getBean(SkillTemplate.class);

            String yaml = """
                    loomspan:
                      connections:
                        tenant:
                          driver: gemini
                          gemini: {vertex-ai: true, project-id: tenant, location: us-central1}
                    """;
            var invalid = adapt("sales", yaml, Map.of(WORKED_SOURCE, WORKED_SKILL.replace("fixture-host:http", "fixture-host:unknown")));
            var report = service.validate(invalid, metadata("sales", null, Map.of()));
            assertThat(report.valid()).isFalse();
            assertThat(report.skills()).isEmpty();
            assertThat(report.issues()).anySatisfy(issue -> {
                assertThat(issue.severity()).isEqualTo(SkillValidationIssue.Severity.ERROR);
                assertThat(issue.sourceName()).isEqualTo(PublicationConfiguration.SOURCE);
                assertThat(issue.fieldPath()).isEqualTo("loomspan.connections.tenant.gemini.credentials-json");
            }).anySatisfy(issue -> {
                assertThat(issue.sourceName()).isEqualTo(WORKED_SOURCE);
                assertThat(issue.skillName()).isEqualTo("sales:read-record");
                assertThat(issue.message()).contains("fixture-host:unknown");
            });

            var staged = service.validate(serving, metadata("sales", null, Map.of()));
            assertThat(staged.valid()).isTrue();
            assertThat(staged.skills()).singleElement().satisfies(skill -> {
                assertThat(skill.namespace()).isEqualTo("sales");
                assertThat(skill.localName()).isEqualTo("read-record");
            });
            assertThat(reloader.snapshot("sales").generationId()).isEqualTo(previous.generationId());
            assertThat(reloader.snapshot("sales").publicationVersion()).contains(VERSION);
            assertThat(template.invoke("sales", "read-record", Map.of())).isEqualTo("west/original");
        });
    }

    @Test
    void publishesExactWorkedValuesAndIsolatesNamespacesOnSameVersionRotation(@TempDir Path directory) throws Exception {
        engine(directory).run(context -> {
            assertThat(context).hasNotFailed();
            var reloader = context.getBean(SkillReloader.class);
            var manager = context.getBean(SkillGenerationManager.class);
            var template = context.getBean(SkillTemplate.class);
            var executor = context.getBean(RecordingExecutor.class);
            var service = withHostExecutor(reloader);

            var sales = workedPack("sales");
            var occurrences = AuthoredYamlVariables.execution(sales.executionYaml(), PublicationConfiguration.SOURCE).discover();
            assertThat(occurrences).hasSize(5);
            assertThat(occurrences).extracting(AuthoredYamlVariables.Occurrence::name)
                    .containsExactly("provider.key", "app.region", "records.url", "app.region", "app.region");
            assertThat(AuthoredYamlVariables.skill(sales.skills().getFirst().yaml(), WORKED_SOURCE).discover())
                    .containsExactly(new AuthoredYamlVariables.Occurrence("records.lookup-path", WORKED_SOURCE, "config.path"));

            var first = values("sales-key", "west", "/" + "$" + "{untouched}");
            service.publish(sales, metadata("sales", VERSION, first));
            var support = workedPack("support");
            service.publish(support, metadata("support", "other-version", values("support-key", "east", "/support")));
            var before = reloader.snapshot("sales");
            var beforeSupport = reloader.snapshot("support");
            assertThat(before.publicationVersion()).contains(VERSION);
            assertThat(manager.active("sales").visibility()).isEqualTo(NamespaceVisibility.PRIVATE);
            assertThat(manager.active("support").visibility()).isEqualTo(NamespaceVisibility.PRIVATE);
            var salesRuntime = manager.active("sales").runtime().properties();
            var salesConnection = salesRuntime.getConnections().get("primary");
            var supportConnection = manager.active("support").runtime().properties().getConnections().get("primary");
            assertThat(salesConnection.getApiKey()).isEqualTo("sales-key");
            assertThat(salesConnection.getHeaders()).containsEntry("X-Region", "west");
            assertThat(supportConnection.getApiKey()).isEqualTo("support-key");
            assertThat(supportConnection.getHeaders()).containsEntry("X-Region", "east");
            assertThat(salesRuntime.getSession().getMissionTimeout()).isEqualTo(Duration.ofSeconds(60));
            assertThat(salesRuntime.getSession().getMaxDepth()).isEqualTo(32);
            assertThat(salesRuntime.getSession().getAttachments().getMaxSize()).satisfies(size ->
                    assertThat(size.toBytes()).isEqualTo(20_971_520));
            assertThat(salesRuntime.getSession().getQuotas().getMaxProviderAttempts()).isEqualTo(192);
            assertThat(salesRuntime.getExecutionTrace().getPersistence().name()).isEqualTo("ONERROR");
            assertThat(template.invoke("support", "read-record", Map.of())).isEqualTo("east/support");

            try (var calls = Executors.newVirtualThreadPerTaskExecutor()) {
                var inFlight = calls.submit(() -> template.invoke("sales", "read-record", Map.of("wait", true)));
                try {
                    assertThat(executor.entered.await(5, TimeUnit.SECONDS)).isTrue();
                    service.publish(sales, metadata("sales", VERSION, values("edited-key", "north", "/edited")));
                    var rotated = reloader.snapshot("sales");
                    assertThat(rotated.generationId()).isNotEqualTo(before.generationId());
                    assertThat(rotated.publicationVersion()).contains(VERSION);
                    assertThat(template.invoke("sales", "read-record", Map.of())).isEqualTo("north/edited");
                    assertThat(reloader.snapshot("support").generationId()).isEqualTo(beforeSupport.generationId());
                } finally {
                    executor.release.countDown();
                }
                assertThat(inFlight.get(5, TimeUnit.SECONDS)).isEqualTo("west/" + "$" + "{untouched}");
            }

            var invocation = executor.invocations.stream()
                    .filter(call -> Boolean.TRUE.equals(call.input().get("wait"))).findFirst().orElseThrow();
            var records = (Map<?, ?>) invocation.consumerExecutorConfig().get("records");
            assertThat(records.get("base-url")).isEqualTo("https://records.example.test");
            assertThat(records.get("headers")).isEqualTo(Map.of("X-Region", "west", "Accept", "application/json"));
            assertThat(records.get("regions")).isEqualTo(List.of(Map.of("values", List.of("west"))));
            assertThat(invocation.skillConfig()).containsEntry("path", "/" + "$" + "{untouched}");

            service.publish(adapt("sales", "loomspan: {}", Map.of()), metadata("sales", "empty", Map.of()));
            var empty = reloader.snapshot("sales");
            assertThat(empty.publicationVersion()).contains("empty");
            assertThat(empty.skills()).isEmpty();
            assertThat(template.invoke("support", "read-record", Map.of())).isEqualTo("east/support");
        });
    }

    @Test
    void rejectsInvalidReplacementAndLeavesThePreviousGenerationServing(@TempDir Path directory) throws Exception {
        engine(directory).run(context -> {
            assertThat(context).hasNotFailed();
            var reloader = context.getBean(SkillReloader.class);
            var service = withHostExecutor(reloader);
            var template = context.getBean(SkillTemplate.class);
            var sales = workedPack("sales");
            var first = values("sales-key", "west", "/original");
            service.publish(sales, metadata("sales", VERSION, first));
            var before = reloader.snapshot("sales");

            assertThat(context.getEnvironment().getProperty("app.region")).isEqualTo("host-third");
            assertThatThrownBy(() -> service.publish(sales, metadata("sales", "rejected",
                    Map.of("provider.key", "sales-key", "records.url", "https://records.example.test",
                            "records.lookup-path", "/original")))).hasMessageContaining("loomspan.connections.primary.headers.X-Region");
            assertThatThrownBy(() -> metadata("sales", "rejected", values("sales-key", " ", "/original")))
                    .hasMessageContaining("variableValues.app.region");
            assertThatThrownBy(() -> service.publish(sales, metadata("sales", "rejected",
                    Map.of("provider.key", "sales-key", "app.region", "west", "records.url", "https://records.example.test",
                            "records.lookup-path", "/original", "unreferenced", "extra")))).hasMessageContaining("unused entry");

            // Framework's loomspan-variable-fixtures/collision, completed as a skill for the host executor.
            var colliding = adapt("sales", "loomspan: {}", Map.of(COLLISION_SOURCE, """
                    name: read-record
                    description: Candidate with conflicting effective keys
                    executor: fixture-host:http
                    config:
                      headers:
                        ? { $var: header.name }
                        : { $var: app.region }
                        X-Region: literal
                    """));
            assertThat(service.validate(colliding, metadata("sales", null, Map.of())).valid()).isTrue();
            assertThatThrownBy(() -> service.publish(colliding,
                    metadata("sales", "rejected", Map.of("header.name", "X-Region", "app.region", "west"))))
                    .isInstanceOf(SkillReloadException.class).hasMessageContaining("key-collision")
                    .hasMessageContaining(COLLISION_SOURCE).hasMessageContaining("config.headers{key#0}")
                    .hasMessageContaining("config.headers.X-Region");

            var wrongCredential = adapt("sales", """
                    loomspan:
                      connections:
                        tenant:
                          driver: gemini
                          gemini: {vertex-ai: true, project-id: tenant, location: us-central1}
                    """, Map.of());
            assertThatThrownBy(() -> service.publish(wrongCredential, metadata("sales", VERSION, Map.of())))
                    .hasMessageContaining("loomspan.connections.tenant.gemini.credentials-json");

            var failure = new SkillReloadException("Simulated publication exception after successful preparation");
            var rejectAtPublish = new PublicationReloaderStub() {
                @Override
                public PreparedSkillUpdate prepare(String namespace, String version, Collection<SkillDocument> documents,
                                                   ExecutionConfiguration configuration, NamespaceVisibility visibility,
                                                   Map<String, String> variableValues) {
                    return reloader.prepare(namespace, version, documents, configuration, visibility, variableValues);
                }

                @Override
                public void publish(PreparedSkillUpdate candidate) {
                    throw failure;
                }
            };
            var failingService = new PublicationService(rejectAtPublish, new TenantVertexPolicy());
            assertThatThrownBy(() -> failingService.publish(sales, metadata("sales", "rejected", first))).isSameAs(failure);

            assertThat(reloader.snapshot("sales").generationId()).isEqualTo(before.generationId());
            assertThat(reloader.snapshot("sales").publicationVersion()).contains(VERSION);
            assertThat(template.invoke("sales", "read-record", Map.of())).isEqualTo("west/original");
        });
    }

    @Test
    void preservesAliasesAndComplexKeysDuringConfigurationComposition(@TempDir Path directory) throws Exception {
        engine(directory).run(context -> {
            assertThat(context).hasNotFailed();
            var reloader = context.getBean(SkillReloader.class);
            var service = withHostExecutor(reloader);
            var yaml = """
                    loomspan:
                      executor-config:
                        sales:
                          targets:
                            records:
                              headers: &headers
                                ? { $var: name }
                                : { $var: region }
                              sibling: *headers
                    """;
            var candidate = adapt("sales", yaml, Map.of(WORKED_SOURCE, WORKED_SKILL));
            service.publish(candidate, metadata("sales", VERSION,
                    Map.of("name", "X-Region", "region", "west", "records.lookup-path", "/aliases")));
            var template = context.getBean(SkillTemplate.class);
            assertThat(template.invoke("sales", "read-record", Map.of())).isEqualTo("west/aliases");
            var invocation = context.getBean(RecordingExecutor.class).invocations.getFirst();
            var records = (Map<?, ?>) invocation.consumerExecutorConfig().get("records");
            assertThat(records.get("headers")).isEqualTo(Map.of("X-Region", "west"));
            assertThat(records.get("sibling")).isEqualTo(Map.of("X-Region", "west"));
        });
    }

    private static ApplicationContextRunner engine(Path directory) throws Exception {
        var startup = directory.resolve("application.yml");
        Files.writeString(startup, "loomspan: {executor-config: {fixture-host: {targets: {}}}}\n");
        return new ApplicationContextRunner()
                .withConfiguration(AutoConfigurations.of(LoomspanJacksonAutoConfiguration.class, LoomspanAutoConfiguration.class,
                        LoomspanAiAutoConfiguration.class))
                .withUserConfiguration(HostExtensions.class)
                .withInitializer(new ConfigDataApplicationContextInitializer())
                .withPropertyValues("spring.config.location=" + startup.toUri(),
                        "loomspan.skills.locations=" + directory.toUri() + "skills/*.yaml",
                        "loomspan.execution-trace.directory=" + directory.resolve("traces"),
                        "app.region=host-third", "provider.key=host-key");
    }

    private static PublicationService withHostExecutor(SkillReloader reloader) {
        try (var host = reloader.prepare("fixture-host", List.of(),
                new ExecutionConfiguration("loomspan: {executor-config: {fixture-host: {targets: {}}}}"),
                NamespaceVisibility.PUBLIC)) {
            reloader.publish(host);
        }
        return new PublicationService(reloader, new TenantVertexPolicy());
    }

    private static Map<String, String> values(String key, String region, String path) {
        return Map.of("provider.key", key, "app.region", region, "records.url", "https://records.example.test",
                "records.lookup-path", path);
    }

    private static PublicationMetadata metadata(String namespace, String version, Map<String, String> values) {
        var metadata = (ObjectNode) JSON.readTree(PublicationControllerTest.metadata(namespace));
        if (version != null) {
            metadata.put("publicationVersion", version);
            var variables = metadata.putObject("variableValues");
            values.forEach(variables::put);
        }
        return PublicationMetadata.parse(metadata, version != null);
    }

    private static AdaptedPublication workedPack(String namespace) throws Exception {
        return adapt(namespace, WORKED_EXECUTION.replace("OWNER", namespace),
                Map.of(WORKED_SOURCE, WORKED_SKILL));
    }

    private static AdaptedPublication adapt(String namespace, String execution, Map<String, String> skills) throws Exception {
        var part = new MockPart("pack", "publication.zip", PublicationControllerTest.pack(namespace, execution, skills));
        part.getHeaders().setContentType(org.springframework.http.MediaType.valueOf("application/zip"));
        return new PublicationZipAdapter().adapt(namespace, part);
    }

    @TestConfiguration(proxyBeanMethods = false)
    @EnableConfigurationProperties
    static class HostExtensions {
        @Bean
        RecordingExecutor recordingExecutor() {
            return new RecordingExecutor();
        }

    }

    static class RecordingExecutor {
        final List<SkillExecutorInvocation> invocations = new CopyOnWriteArrayList<>();
        final CountDownLatch entered = new CountDownLatch(1);
        final CountDownLatch release = new CountDownLatch(1);

        @SkillExecutor(namespace = "fixture-host", name = "http", config = "targets")
        public String execute(SkillExecutorInvocation invocation) throws InterruptedException {
            invocations.add(invocation);
            if (Boolean.TRUE.equals(invocation.input().get("wait"))) {
                entered.countDown();
                if (!release.await(10, TimeUnit.SECONDS)) {
                    throw new IllegalStateException("Timed out waiting to release the fixture invocation");
                }
            }
            var records = (Map<?, ?>) invocation.consumerExecutorConfig().get("records");
            var headers = (Map<?, ?>) records.get("headers");
            return headers.get("X-Region") + (String) invocation.skillConfig().get("path");
        }
    }
}

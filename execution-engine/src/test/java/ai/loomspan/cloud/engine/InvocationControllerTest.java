package ai.loomspan.cloud.engine;

import ai.loomspan.api.SkillException;
import ai.loomspan.api.SkillExecutionView;
import ai.loomspan.api.SkillExecutor;
import ai.loomspan.api.SkillTemplate;
import ai.loomspan.api.extensions.v1.SkillExecutorInvocation;
import ai.loomspan.cloud.engine.fixture.MilestoneExecutorConfiguration.MilestoneExecutor;
import ai.loomspan.internal.core.FrameworkExecutionLifecycle;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;
import org.springframework.beans.factory.support.StaticListableBeanFactory;
import org.springframework.boot.test.context.TestConfiguration;
import org.springframework.context.annotation.Bean;
import org.springframework.security.authentication.UsernamePasswordAuthenticationToken;
import org.springframework.security.core.context.SecurityContextHolder;
import org.springframework.security.core.context.SecurityContextHolderStrategy;
import org.springframework.web.client.HttpServerErrorException;

import java.net.http.HttpResponse;
import java.nio.file.Path;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.RejectedExecutionException;
import java.util.concurrent.TimeUnit;
import java.util.function.Consumer;

import static ai.loomspan.cloud.engine.InvocationTestEngine.CALLER;
import static ai.loomspan.cloud.engine.InvocationTestEngine.JSON;
import static ai.loomspan.cloud.engine.InvocationTestEngine.SKILL;
import static ai.loomspan.cloud.engine.InvocationTestEngine.invocation;
import static org.assertj.core.api.Assertions.assertThat;

class InvocationControllerTest {
    private static EngineClientIdentityTest.AuthorizedFixture tls;

    @BeforeAll
    static void credentials(@TempDir Path directory) throws Exception {
        tls = EngineClientIdentityTest.authorizedFixture(directory);
    }

    @Test
    void originalPackReturnsFirstPing() throws Exception {
        try (var engine = new InvocationTestEngine(tls)) {
            engine.publishLeaf("first");
            var response = engine.invoke(invocation(SKILL, "{\"text\":\"ping\"}", CALLER, false));
            assertThat(response.statusCode()).as(response.body()).isEqualTo(200);
            assertThat(JSON.readTree(response.body())).isEqualTo(JSON.readTree("{\"result\":\"first/ping\"}"));
        }
    }

    @Test
    void validatesCallerAndPreservesDiagnosticsFromRealInvocations() throws Exception {
        try (var engine = new InvocationTestEngine(tls)) {
            engine.publish("roles", "loomspan: {}", Map.of(
                    "loomspan/skills/echo-release.yaml", InvocationTestEngine.leaf("restricted") + "rbac_roles: [reviewer]\n"));
            for (String invalidCaller : List.of("null", "{}", "{\"issuer\":\"\",\"subject\":\"user\",\"roles\":[]}",
                    "{\"issuer\":\"cloud\",\"subject\":\" \",\"roles\":[]}",
                    "{\"issuer\":\"cloud\",\"subject\":\"user\",\"roles\":null}",
                    "{\"issuer\":\"cloud\",\"subject\":\"user\",\"roles\":[false]}",
                    "{\"issuer\":\"cloud\",\"issuer\":\"other\",\"subject\":\"user\",\"roles\":[]}")) {
                var response = engine.invoke(invocation(SKILL, "{\"text\":\"ping\"}", invalidCaller, false));
                assertThat(response.statusCode()).as(response.body()).isEqualTo(400);
                assertThat(response.headers().firstValue("Content-Type")).hasValueSatisfying(
                        contentType -> assertThat(contentType).startsWith("application/json"));
                assertThat(JSON.readTree(response.body()).has("debug")).isTrue();
            }
            assertThat(engine.invoke(invocation(SKILL, "{}", CALLER, false) + " {}").statusCode()).isEqualTo(400);
            assertThat(engine.invoke(invocation(SKILL, "{\"text\":\"ping\"}", CALLER, false)).statusCode())
                    .isEqualTo(403);
            assertThat(engine.context.getBean(MilestoneExecutor.class).invocations()).isZero();
            for (String role : List.of("reviewer", "ROLE_reviewer")) {
                var caller = CALLER.replace("[]", "[\"" + role + "\"]");
                var response = engine.invoke(invocation(SKILL, "{\"text\":\"ping\"}", caller, true));
                assertThat(response.statusCode()).as(response.body()).isEqualTo(200);
                var body = JSON.readTree(response.body());
                assertThat(body.get("result").stringValue()).isEqualTo("restricted/ping");
                assertThat(body.at("/executionView/sessionId").stringValue()).isNotBlank();
                assertThat(body.at("/executionView/events").size()).isPositive();
            }
            var caller = CALLER.replace("[]", "[\"reviewer\"]");
            var failure = engine.invoke(invocation(SKILL, "{\"text\":\"fail\"}", caller, true));
            assertThat(failure.statusCode()).isEqualTo(500);
            var failed = JSON.readTree(failure.body());
            assertThat(failed.path("message").stringValue()).isNotBlank();
            assertThat(failed.path("debug").stringValue()).contains("SkillException", "Caused by:",
                    "Milestone executor received fail");
            assertThat(failed.at("/executionView/events").size()).isPositive();
            var noObserver = JSON.readTree(engine.invoke(invocation(SKILL, "{\"text\":\"fail\"}", caller, false)).body());
            assertThat(noObserver.path("message")).isEqualTo(failed.path("message"));
            assertThat(noObserver.has("executionView")).isFalse();

            var invalid = engine.invoke(invocation(SKILL, "{}", caller, true));
            assertThat(invalid.statusCode()).as(invalid.body()).isEqualTo(400);
            var validation = JSON.readTree(invalid.body());
            assertThat(validation.at("/issues/0/code").stringValue()).isEqualTo("required");
            assertThat(validation.at("/diagnosis/issues/0/code").stringValue()).isEqualTo("required");
            assertThat(validation.path("executionView").isNull()).isTrue();
        }
    }

    @Test
    void admissionClosureRejectsAnInTransitRequestWhileAStartedProviderFailureDoesNot() throws Exception {
        try (var engine = new InvocationTestEngine(tls, Probes.class)) {
            installProbe(engine);
            var provider = engine.invoke(invocation("probe", "{\"text\":\"provider\"}", CALLER, true));
            assertThat(provider.statusCode()).as(provider.body()).isEqualTo(500);
            assertThat(JSON.readTree(provider.body()).path("debug").stringValue()).contains("503", "Provider unavailable");
            var encoded = engine.invoke(invocation("probe", "{\"text\":\"json\"}", CALLER, false));
            assertThat(encoded.statusCode()).isEqualTo(200);
            assertThat(JSON.readTree(encoded.body()).get("result").stringValue()).isEqualTo("{\"inner\":true}");
            var gates = engine.context.getBean(ProbeExecutor.class);
            try (var transit = engine.sendInTransit(invocation("probe", "{\"text\":\"hold\"}", CALLER, true))) {
                engine.context.getBean(FrameworkExecutionLifecycle.class).closeAdmission();
                var dropped = transit.deliver();
                assertThat(dropped.statusCode()).as(dropped.body()).isEqualTo(503);
                assertThat(JSON.readTree(dropped.body()).path("executionView").isNull()).isTrue();
                assertThat(gates.entered.getCount()).isEqualTo(1);
            }
        }
    }

    @Test
    void preservesExactDecimalInputBeforeFrameworkAdmission() throws Exception {
        try (var engine = new InvocationTestEngine(tls, Probes.class)) {
            installProbe(engine);
            var response = engine.invoke(invocation("probe",
                    "{\"text\":\"decimal\",\"amount\":9007199254740993.25}", CALLER, false));
            assertThat(response.statusCode()).as(response.body()).isEqualTo(200);
            assertThat(JSON.readTree(response.body()).get("result").stringValue()).isEqualTo("9007199254740993.25");
        }
    }

    @Test
    void clientCancellationDoesNotCancelAdmittedExecution() throws Exception {
        try (var engine = new InvocationTestEngine(tls, Probes.class)) {
            installProbe(engine);
            var gates = engine.context.getBean(ProbeExecutor.class);
            var response = engine.client.sendAsync(engine.invokeRequest(java.net.http.HttpRequest.BodyPublishers.ofString(
                    invocation("probe", "{\"text\":\"hold\"}", CALLER, false))), HttpResponse.BodyHandlers.ofString());
            try {
                assertThat(gates.entered.await(10, TimeUnit.SECONDS)).isTrue();
                assertThat(response.cancel(true)).isTrue();
            } finally {
                gates.release.countDown();
            }
            assertThat(gates.finish.await(10, TimeUnit.SECONDS)).isTrue();
        }
    }

    @Test
    void serviceScopesIdentitySelectsObserverOverloadAndRestoresPriorContext() {
        var strategy = SecurityContextHolder.getContextHolderStrategy();
        var prior = strategy.createEmptyContext();
        prior.setAuthentication(UsernamePasswordAuthenticationToken.unauthenticated("previous", null));
        var caller = new InvocationService.Caller("issuer:unaltered", "subject:unaltered", List.of("reader", "ROLE_owner"));
        Map<String, Object> input = Map.of("text", "ping");
        var view = new SkillExecutionView("fixture-session", List.of());
        var template = new RecordingTemplate(() -> {
            var auth = strategy.getContext().getAuthentication();
            assertThat(auth.getPrincipal()).isEqualTo(new InvocationService.InvocationPrincipal(caller.issuer(), caller.subject()));
            assertThat(auth.getAuthorities()).extracting(org.springframework.security.core.GrantedAuthority::getAuthority)
                    .containsExactlyInAnyOrder("ROLE_reader", "ROLE_owner");
        }, view);
        var service = new InvocationService(template, new StaticListableBeanFactory(Map.of("strategy", strategy))
                .getBeanProvider(SecurityContextHolderStrategy.class));
        var before = strategy.getContext();
        strategy.setContext(prior);
        try {
            assertThat(service.invoke("exact", "local", input, caller, false).result()).isEqualTo("{\"inner\":true}");
            assertThat(strategy.getContext()).isSameAs(prior);
            var failure = service.invoke("exact", "local", input, caller, true);
            assertThat(failure.view()).isSameAs(view);
            assertThat(failure.failure()).hasMessage("fixture failure");
            assertThat(strategy.getContext()).isSameAs(prior);
            assertThat(template.withoutObserver).isEqualTo(1);
            assertThat(template.withObserver).isEqualTo(1);
        } finally {
            strategy.setContext(before);
        }
    }

    private static final class RecordingTemplate implements SkillTemplate {
        private final Runnable inspect;
        private final SkillExecutionView view;
        private int withObserver;
        private int withoutObserver;

        private RecordingTemplate(Runnable inspect, SkillExecutionView view) {
            this.inspect = inspect;
            this.view = view;
        }

        @Override
        public String invoke(String namespace, String skill, Map<String, Object> input) {
            assertThat(namespace).isEqualTo("exact");
            assertThat(skill).isEqualTo("local");
            assertThat(input).containsExactlyEntriesOf(Map.of("text", "ping"));
            inspect.run();
            withoutObserver++;
            return "{\"inner\":true}";
        }

        @Override
        public String invoke(String namespace, String skill, Map<String, Object> input, Consumer<SkillExecutionView> observer) {
            assertThat(namespace).isEqualTo("exact");
            assertThat(skill).isEqualTo("local");
            assertThat(input).containsExactlyEntriesOf(Map.of("text", "ping"));
            inspect.run();
            withObserver++;
            observer.accept(view);
            throw new IllegalStateException("fixture failure");
        }

        @Override
        public String invoke(String namespace, String skill, Object input) {
            throw new AssertionError("Unexpected object overload");
        }

        @Override
        public String invoke(String namespace, String skill, Object input, Consumer<SkillExecutionView> observer) {
            throw new AssertionError("Unexpected object overload");
        }

        @Override
        public void validate(String namespace, String skill, Object input) {
            throw new AssertionError("Unexpected invocation validation");
        }

        @Override
        public void validate(String namespace, String skill, Map<String, Object> input) {
            throw new AssertionError("Unexpected invocation validation");
        }
    }

    @Test
    void failureEnvelopePreservesNestedAndSuppressedCausesAndRejectsDownstreamRejectionSignal() {
        var cause = new RejectedExecutionException("Loomspan framework is shutting down; new root work is not accepted");
        var failure = new SkillException("original", cause);
        failure.addSuppressed(new IllegalStateException("secondary"));
        assertThat(InvocationAdmissionRejection.isConfirmed(failure)).isFalse();
        var envelope = InvocationController.failureBody(failure);
        assertThat(envelope.get("message")).isEqualTo("original");
        assertThat((String) envelope.get("debug")).contains("SkillException: original", "Suppressed:", "secondary",
                "Caused by:", "RejectedExecutionException", "\tat ");
    }

    private static void installProbe(InvocationTestEngine engine) throws Exception {
        String yaml = InvocationTestEngine.leaf("unused").replace("name: echo-release", "name: probe")
                .replace("executor: milestone-host:echo-release", "executor: milestone-host:signal");
        engine.publish("probe", "loomspan: {}", Map.of("loomspan/skills/probe.yaml", yaml));
    }

    @TestConfiguration(proxyBeanMethods = false)
    static class Probes {
        @Bean
        ProbeExecutor probeExecutor() {
            return new ProbeExecutor();
        }
    }

    public static class ProbeExecutor {
        private final CountDownLatch entered = new CountDownLatch(1);
        private final CountDownLatch release = new CountDownLatch(1);
        private final CountDownLatch finish = new CountDownLatch(1);

        @SkillExecutor(namespace = "milestone-host", name = "signal")
        public String execute(SkillExecutorInvocation invocation) throws InterruptedException {
            var text = invocation.input().get("text");
            if ("provider".equals(text)) {
                throw new HttpServerErrorException(org.springframework.http.HttpStatus.SERVICE_UNAVAILABLE,
                        "Provider unavailable");
            }
            if ("json".equals(text)) {
                return "{\"inner\":true}";
            }
            if ("decimal".equals(text)) {
                return invocation.input().get("amount").toString();
            }
            entered.countDown();
            if (!release.await(10, TimeUnit.SECONDS) || invocation.isCancelled()) {
                throw new IllegalStateException("Admitted execution did not complete");
            }
            finish.countDown();
            return "finished";
        }
    }
}

package ai.loomspan.cloud.engine;

import com.sun.net.httpserver.HttpServer;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.io.IOException;
import java.net.InetSocketAddress;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.nio.file.Path;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;

import static ai.loomspan.cloud.engine.InvocationTestEngine.CALLER;
import static ai.loomspan.cloud.engine.InvocationTestEngine.JSON;
import static ai.loomspan.cloud.engine.InvocationTestEngine.invocation;
import static org.assertj.core.api.Assertions.assertThat;

class InvocationCutoverTest {
    @Test
    void onlyAdmittedRequestRetainsItsReachableChildAcrossPublication(@TempDir Path directory) throws Exception {
        var tls = EngineClientIdentityTest.authorizedFixture(directory);
        var plannerReached = new CountDownLatch(1);
        var releaseOldPlanner = new CountDownLatch(1);
        var firstResponse = new AtomicBoolean(true);
        byte[] plannerResponse = response("""
                {"capabilityName":"milestone:parent","createdAt":"2026-11-20T00:00:00Z","status":"VALID",
                 "tasks":[{"taskId":"one","title":"one","status":"PENDING","capabilityName":"milestone:echo-release",
                           "intent":"Forward","dependsOn":[],"expectedOutputs":[],"parallelGroup":null,"note":""}]}
                """);
        byte[] stepResponse = response("""
                {"stepAction":"CALL_TOOL","taskId":"one","toolName":"milestone:echo-release","toolArguments":{}}
                """);
        var planner = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        try (var workers = Executors.newVirtualThreadPerTaskExecutor()) {
            planner.setExecutor(workers);
            planner.createContext("/v1/responses", exchange -> {
                try (exchange) {
                    String request = new String(exchange.getRequestBody().readAllBytes(), StandardCharsets.UTF_8);
                    boolean planning = request.contains("Create an ordered flight plan");
                    if (planning && firstResponse.getAndSet(false)) {
                        plannerReached.countDown();
                        try {
                            if (!releaseOldPlanner.await(20, TimeUnit.SECONDS)) {
                                exchange.sendResponseHeaders(500, -1);
                                return;
                            }
                        } catch (InterruptedException interrupted) {
                            Thread.currentThread().interrupt();
                            throw new IOException(interrupted);
                        }
                    }
                    byte[] body = planning ? plannerResponse : stepResponse;
                    exchange.getResponseHeaders().set("Content-Type", "application/json");
                    exchange.sendResponseHeaders(200, body.length);
                    exchange.getResponseBody().write(body);
                }
            });
            planner.start();
            try (var engine = new InvocationTestEngine(tls)) {
                try {
                    publish(engine, planner.getAddress().getPort(), "first");
                    var payload = invocation("parent", "{\"text\":\"ping\"}", CALLER, false);
                    var old = engine.client.sendAsync(engine.invokeRequest(HttpRequest.BodyPublishers.ofString(payload)),
                            HttpResponse.BodyHandlers.ofString());
                    assertThat(plannerReached.await(10, TimeUnit.SECONDS)).isTrue();
                    try (var transit = engine.sendInTransit(payload)) {
                        publish(engine, planner.getAddress().getPort(), "restored");
                        assertResult(transit.deliver(), "restored/ping");
                        assertResult(engine.invoke(payload), "restored/ping");
                        assertThat(old).isNotCompleted();
                        releaseOldPlanner.countDown();
                        assertResult(old.get(10, TimeUnit.SECONDS), "first/ping");
                    }
                } finally {
                    releaseOldPlanner.countDown();
                }
            } finally {
                planner.stop(0);
            }
        }
    }

    private static void publish(InvocationTestEngine engine, int port, String release) throws Exception {
        String parent = """
                name: parent
                description: Forward input to the %s child
                model: local
                planning_mode: true
                max_steps: 1
                input_schema:
                  type: object
                  required: [text]
                  properties:
                    text: {type: string}
                allowed_skills:
                  - name: milestone:echo-release
                    required: true
                    max_tasks: 1
                    input_bindings:
                      /text: {from: input, path: /text}
                output_from: {skill: milestone:echo-release}
                """.formatted(release);
        String configuration = """
                loomspan:
                  connections:
                    local: {driver: openai, base-url: 'http://127.0.0.1:%s/v1', api-key: fixture}
                  models:
                    local: {connection: local, provider-model: fixture}
                """.formatted(port);
        engine.publish(release, configuration, Map.of("loomspan/skills/parent.yaml", parent,
                "loomspan/skills/echo-release.yaml", InvocationTestEngine.leaf(release)));
    }

    private static byte[] response(String content) {
        return JSON.writeValueAsString(Map.of("id", "fixture", "object", "response", "created_at", 1, "model", "fixture",
                "status", "completed", "output", List.of(Map.of("id", "msg_0", "type", "message", "role", "assistant",
                        "status", "completed", "content", List.of(Map.of("type", "output_text", "text", content,
                                "annotations", List.of())))),
                "usage", Map.of("input_tokens", 1, "output_tokens", 1, "total_tokens", 2))).getBytes(StandardCharsets.UTF_8);
    }

    private static void assertResult(HttpResponse<String> response, String value) {
        assertThat(response.statusCode()).as(response.body()).isEqualTo(200);
        assertThat(JSON.readTree(response.body())).isEqualTo(JSON.createObjectNode().put("result", value));
    }
}

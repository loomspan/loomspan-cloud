package ai.loomspan.cloud.engine.fixture;

import ai.loomspan.api.ExecutionConfiguration;
import ai.loomspan.api.NamespaceVisibility;
import ai.loomspan.api.SkillExecutor;
import ai.loomspan.api.SkillReloader;
import ai.loomspan.api.extensions.v1.SkillExecutorInvocation;
import org.springframework.context.SmartLifecycle;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Profile;

import java.util.List;
import java.util.Map;
import java.util.concurrent.atomic.AtomicInteger;

@Configuration(proxyBeanMethods = false)
@Profile("milestone-fixture")
public class MilestoneExecutorConfiguration {
    @Bean
    public MilestoneExecutor milestoneExecutor() {
        return new MilestoneExecutor();
    }

    @Bean
    SmartLifecycle milestoneHostGeneration(SkillReloader reloader) {
        return new SmartLifecycle() {
            private boolean running;

            @Override
            public void start() {
                try (var candidate = reloader.prepare("milestone-host", List.of(),
                        new ExecutionConfiguration("loomspan: {}"), NamespaceVisibility.PUBLIC, Map.of())) {
                    reloader.publish(candidate);
                }
                running = true;
            }

            @Override
            public int getPhase() {
                return 0;
            }

            @Override
            public void stop() {
                running = false;
            }

            @Override
            public boolean isRunning() {
                return running;
            }
        };
    }

    public static class MilestoneExecutor {
        private final AtomicInteger invocations = new AtomicInteger();

        public int invocations() {
            return invocations.get();
        }

        @SkillExecutor(namespace = "milestone-host", name = "echo-release")
        public String echo(SkillExecutorInvocation invocation) {
            invocations.incrementAndGet();
            var text = (String) invocation.input().get("text");
            if ("fail".equals(text)) {
                throw new IllegalStateException("Milestone executor received fail");
            }
            return invocation.skillConfig().get("release") + "/" + text;
        }
    }
}

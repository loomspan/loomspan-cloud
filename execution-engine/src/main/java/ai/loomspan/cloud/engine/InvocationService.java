package ai.loomspan.cloud.engine;

import ai.loomspan.api.SkillExecutionView;
import ai.loomspan.api.SkillTemplate;
import org.springframework.beans.factory.ObjectProvider;
import org.springframework.security.authentication.UsernamePasswordAuthenticationToken;
import org.springframework.security.core.authority.SimpleGrantedAuthority;
import org.springframework.security.core.context.SecurityContextHolder;
import org.springframework.security.core.context.SecurityContextHolderStrategy;
import org.springframework.stereotype.Component;

import java.security.Principal;
import java.util.List;
import java.util.Map;
import java.util.concurrent.atomic.AtomicReference;

@Component
final class InvocationService {
    private final SkillTemplate template;
    private final SecurityContextHolderStrategy security;

    InvocationService(SkillTemplate template, ObjectProvider<SecurityContextHolderStrategy> security) {
        this.template = template;
        this.security = security.getIfAvailable(SecurityContextHolder::getContextHolderStrategy);
    }

    Result invoke(String namespace, String skill, Map<String, Object> input, Caller caller, boolean includeView) {
        var principal = new InvocationPrincipal(caller.issuer(), caller.subject());
        var authorities = caller.roles().stream()
                .map(role -> new SimpleGrantedAuthority(role.startsWith("ROLE_") ? role : "ROLE_" + role))
                .toList();
        var prior = security.getContext();
        var callerContext = security.createEmptyContext();
        callerContext.setAuthentication(UsernamePasswordAuthenticationToken.authenticated(principal, null, authorities));
        security.setContext(callerContext);
        var view = new AtomicReference<SkillExecutionView>();
        try {
            String result = includeView ? template.invoke(namespace, skill, input, view::set)
                    : template.invoke(namespace, skill, input);
            return new Result(result, view.get(), null);
        } catch (RuntimeException failure) {
            return new Result(null, view.get(), failure);
        } finally {
            security.setContext(prior);
        }
    }

    record Caller(String issuer, String subject, List<String> roles) {}

    record Result(String result, SkillExecutionView view, RuntimeException failure) {}

    record InvocationPrincipal(String issuer, String subject) implements Principal {
        @Override
        public String getName() {
            return issuer.length() + ":" + issuer + subject;
        }
    }
}

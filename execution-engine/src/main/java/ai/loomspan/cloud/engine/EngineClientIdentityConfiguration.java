package ai.loomspan.cloud.engine;

import jakarta.servlet.DispatcherType;
import jakarta.servlet.Filter;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.web.servlet.FilterRegistrationBean;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.core.Ordered;

import java.security.cert.CertificateException;
import java.security.cert.X509Certificate;
import java.util.EnumSet;

@Configuration(proxyBeanMethods = false)
class EngineClientIdentityConfiguration {
    static final String MANAGEMENT_CLIENT_IDENTITY = "spiffe://loomspan-cloud/management/backend";
    private static final String CERTIFICATES_ATTRIBUTE = "jakarta.servlet.request.X509Certificate";
    private static final int URI_GENERAL_NAME = 6;

    @Bean
    FilterRegistrationBean<Filter> engineClientIdentityFilter(
            @Value("${loomspan.cloud.engine.management-client-uri:" + MANAGEMENT_CLIENT_IDENTITY + "}")
            String expectedIdentity) {
        if (expectedIdentity.isBlank()) {
            throw new IllegalArgumentException("loomspan.cloud.engine.management-client-uri is required");
        }
        Filter filter = (request, response, chain) -> {
            var http = (HttpServletRequest) request;
            if (!hasManagementIdentity(http, expectedIdentity)) {
                var rejected = (HttpServletResponse) response;
                rejected.setStatus(HttpServletResponse.SC_FORBIDDEN);
                rejected.setContentType("text/plain;charset=UTF-8");
                rejected.getWriter().write("Authenticated Loomspan management identity required");
                return;
            }
            chain.doFilter(request, response);
        };

        var registration = new FilterRegistrationBean<>(filter);
        registration.setName("LoomspanCloudEngineClientIdentityFilter");
        registration.addUrlPatterns("/internal/v1", "/internal/v1/*",
                "/_loomspan/observability/v1", "/_loomspan/observability/v1/*");
        registration.setDispatcherTypes(EnumSet.allOf(DispatcherType.class));
        registration.setAsyncSupported(true);
        registration.setOrder(Ordered.HIGHEST_PRECEDENCE);
        return registration;
    }

    private static boolean hasManagementIdentity(HttpServletRequest request, String identity) {
        if (!request.isSecure() || !(request.getAttribute(CERTIFICATES_ATTRIBUTE) instanceof X509Certificate[] chain)
                || chain.length == 0 || chain[0] == null) {
            return false;
        }
        try {
            var leaf = chain[0];
            leaf.checkValidity();
            var names = leaf.getSubjectAlternativeNames();
            return names != null && names.stream().anyMatch(name ->
                    name.size() == 2 && Integer.valueOf(URI_GENERAL_NAME).equals(name.get(0))
                            && identity.equals(name.get(1)));
        } catch (CertificateException ex) {
            return false;
        }
    }
}

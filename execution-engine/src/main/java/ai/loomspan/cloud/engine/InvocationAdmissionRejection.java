package ai.loomspan.cloud.engine;

import ai.loomspan.api.SkillException;

import java.util.concurrent.RejectedExecutionException;

/** Matches the pinned starter's two pre-session root rejection sites. */
final class InvocationAdmissionRejection {
    private InvocationAdmissionRejection() {}

    static boolean isConfirmed(RuntimeException failure) {
        if (failure.getClass() != SkillException.class
                || !(failure.getCause() instanceof RejectedExecutionException rejection)) {
            return false;
        }
        var stack = rejection.getStackTrace();
        if (stack.length == 0) {
            return false;
        }
        var origin = stack[0];
        return (origin.getClassName().equals("ai.loomspan.internal.core.FrameworkExecutionLifecycle")
                && origin.getMethodName().equals("admitRoot")
                && "Loomspan framework is shutting down; new root work is not accepted".equals(rejection.getMessage()))
                || (origin.getClassName().equals("ai.loomspan.internal.core.LoomspanSessionRunner")
                && origin.getMethodName().equals("executeAdmittedRoot")
                && "Loomspan invocation admission is no longer executable".equals(rejection.getMessage()));
    }
}

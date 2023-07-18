package rideshare;

import io.otel.pyroscope.shadow.javaagent.api.Logger;
import org.slf4j.LoggerFactory;

class PyroscopeLog4jLogger implements Logger {
    org.slf4j.Logger pyroscopeLogger = LoggerFactory.getLogger("pyroscope");

    @Override
    public void log(Level l, String msg, Object... args) {
        String m = String.format(msg, args);
        switch (l) {
            case INFO:
                pyroscopeLogger.info(m);
                break;
            case DEBUG:
                pyroscopeLogger.debug(m);
                break;
            case WARN:
                pyroscopeLogger.warn(m);
                break;
            case ERROR:
                pyroscopeLogger.error(m);
                break;
            default:
                break;
        }
    }
}

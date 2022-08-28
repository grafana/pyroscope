package rideshare;

import io.pyroscope.http.Format;
import io.pyroscope.javaagent.EventType;
import io.pyroscope.javaagent.PyroscopeAgent;
import io.pyroscope.javaagent.api.Logger;
import io.pyroscope.javaagent.config.Config;
import io.pyroscope.labels.Pyroscope;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

import java.util.Map;

@SpringBootApplication
public class Main {

    public static final String APP_NAME = "ride-sharing-app-java";

    public static void main(String[] args) {
        configurePyroscope();
        SpringApplication.run(Main.class, args);
    }

    private static void configurePyroscope() {
        Config config = new Config.Builder()
                .setApplicationName(APP_NAME)
                .setProfilingEvent(EventType.ITIMER)
                .setFormat(Format.JFR)
                .setLogLevel(Logger.Level.DEBUG)
                .setServerAddress(System.getenv("PYROSCOPE_SERVER_ADDRESS"))
                .build();
        PyroscopeAgent.start(
                new PyroscopeAgent.Options.Builder(config)
                        .setLogger(new PyroscopeLog4jLogger())
                        .build()
        );
        Pyroscope.setStaticLabels(Map.of("region", System.getenv("REGION")));
    }

}

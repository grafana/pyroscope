package rideshare;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

import java.util.Map;

@SpringBootApplication
public class Main {

    public static final String APP_NAME = "ride-sharing-app-java";

    public static void main(String[] args) {
        SpringApplication.run(Main.class, args);
    }

}

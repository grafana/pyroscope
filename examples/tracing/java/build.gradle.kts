plugins {
    id("java")
    id("org.springframework.boot") version "2.7.0"
    id("io.spring.dependency-management") version "1.0.11.RELEASE"
}

group = "org.example"
version = "1.0-SNAPSHOT"

repositories {
    mavenCentral()
}

dependencies {
    implementation("io.pyroscope:otel:1.0.5")

    // Spring Boot
    implementation("org.springframework.boot:spring-boot-starter-web")

    // OTel SDK (programmatic setup)
    implementation(platform("io.opentelemetry:opentelemetry-bom:1.44.1"))
    implementation("io.opentelemetry:opentelemetry-api")
    implementation("io.opentelemetry:opentelemetry-sdk")
    implementation("io.opentelemetry:opentelemetry-exporter-otlp")
    implementation("io.opentelemetry.semconv:opentelemetry-semconv:1.29.0-alpha")

    // Spring Boot OTel integration (without javaagent — instruments HTTP server)
    implementation(platform("io.opentelemetry.instrumentation:opentelemetry-instrumentation-bom:2.10.0"))
    implementation("io.opentelemetry.instrumentation:opentelemetry-spring-boot-starter")

    testImplementation("org.junit.jupiter:junit-jupiter-api:5.8.2")
    testRuntimeOnly("org.junit.jupiter:junit-jupiter-engine:5.8.2")
}

tasks.getByName<Test>("test") {
    useJUnitPlatform()
}

plugins {
    id("java")
    id("org.springframework.boot") version "2.7.1"
    id("io.spring.dependency-management") version "1.0.11.RELEASE"
}

group = "org.example"
version = "1.0-SNAPSHOT"

repositories {
    mavenCentral()
    maven("https://s01.oss.sonatype.org/content/repositories/snapshots/")
    maven("https://repo.spring.io/snapshot")
    maven("https://repo.spring.io/milestone")
    maven("https://repo.spring.io/release")

}

dependencies {
    implementation("io.pyroscope:otel:0.10.1.3")
    implementation(platform("io.opentelemetry:opentelemetry-bom:1.15.0"))
    implementation("io.opentelemetry:opentelemetry-api")
    implementation("io.opentelemetry:opentelemetry-sdk")
    implementation("io.opentelemetry:opentelemetry-exporter-jaeger")
    implementation("org.springframework.boot:spring-boot-starter-web")
    implementation("org.springframework.cloud:spring-cloud-starter-sleuth") {
        exclude(group = "org.springframework.cloud", module = "spring-cloud-sleuth-brave")
    }
    implementation("org.springframework.cloud:spring-cloud-sleuth-otel-autoconfigure")


    testImplementation("org.junit.jupiter:junit-jupiter-api:5.8.2")
    testRuntimeOnly("org.junit.jupiter:junit-jupiter-engine:5.8.2")
}
dependencyManagement {
    imports {
        mavenBom("org.springframework.cloud:spring-cloud-dependencies:2021.0.3")
        mavenBom("org.springframework.cloud:spring-cloud-sleuth-otel-dependencies:1.1.0-M7")
    }
}

tasks.getByName<Test>("test") {
    useJUnitPlatform()
}

tasks.create<Copy>("getDeps") {
    from(configurations.getByName("compileClasspath"))
    into("compileClasspath/")
}

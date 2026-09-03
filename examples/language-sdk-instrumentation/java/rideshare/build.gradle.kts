plugins {
    id("java")
    id("org.springframework.boot") version "3.3.13"
    id("io.spring.dependency-management") version "1.1.6"
}

group = "org.example"
version = "1.0-SNAPSHOT"

repositories {
    mavenCentral()
}

ext["tomcat.version"] = "10.1.55"
ext["jackson-bom.version"] = "2.21.4"

dependencies {
    implementation("io.pyroscope:agent:2.9.1")
    implementation("org.jetbrains:annotations:26.0.2")
    implementation("org.springframework.boot:spring-boot-starter-web")
    testImplementation("org.junit.jupiter:junit-jupiter-api:5.8.2")
    testRuntimeOnly("org.junit.jupiter:junit-jupiter-engine:5.8.2")
}

tasks.getByName<Test>("test") {
    useJUnitPlatform()
}

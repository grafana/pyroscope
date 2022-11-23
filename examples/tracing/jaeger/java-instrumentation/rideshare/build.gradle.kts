plugins {
    id("java")
    id("org.springframework.boot") version "2.7.1"
    id("io.spring.dependency-management") version "1.0.11.RELEASE"
}

group = "org.example"
version = "1.0-SNAPSHOT"

repositories {
    mavenCentral()
    maven("https://s01.oss.sonatype.org/content/repositories/snapshots/") //todo replace when otel is not shnapshot
    maven("https://repo.spring.io/snapshot")
    maven("https://repo.spring.io/milestone")
    maven("https://repo.spring.io/release")

}

dependencies {
    implementation("org.springframework.boot:spring-boot-starter-web")
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

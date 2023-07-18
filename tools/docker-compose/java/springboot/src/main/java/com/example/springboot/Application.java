package com.example.springboot;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.ApplicationContext;

@SpringBootApplication
public class Application {

	public static void main(String[] args) {
		// run some background worload
		Thread t = new Thread(new Something());
		t.start();

		ApplicationContext ctx = SpringApplication.run(Application.class, args);
	}

}

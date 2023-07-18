package com.example;

import java.io.File;
import java.io.FileWriter;

public class Something extends Thread {

    public Something() {
        super();
    }

    @Override
    public void run() {
        doSomething();
    }

    private static void doSomething() {
        try {
            int i = 0;
            for (;;) {
                i++;
                if (i % 3 == 0) {
                    funcFoo();
                }
                funcBar();
                Thread.sleep(10);
            }
        } catch (Exception e) {
            e.printStackTrace();
        }
    }

    private static void funcFoo() {
        funcBuzz();
    }

    private static void funcBar() {
        funcBaz();
    }

    private static void funcBaz() {
        try {
            File f = File.createTempFile("foo", "bar");
            FileWriter w = new FileWriter(f);
            w.write("hello");
            w.close();
            f.delete();
        } catch (Exception e) {
            e.printStackTrace();
        }

    }

    private static void funcBuzz() {
        try {
            File.listRoots();
        } catch (Exception e) {
            e.printStackTrace();
        }
    }

}

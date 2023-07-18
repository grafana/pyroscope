package com.example;

import java.net.InetSocketAddress;
import com.sun.net.httpserver.HttpServer;
import jpprof.PprofHttpHandler;

public class Main {

    public static void main(String[] args) throws Exception {
        // run some background worload
        var t = new Thread(new Something());
        t.start();

        var server = HttpServer.create(new InetSocketAddress(8080), 0);
        server.createContext("/", new PprofHttpHandler());
        server.start();
    }

}

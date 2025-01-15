package org.example.pyrovertx;

import io.vertx.core.AbstractVerticle;
import io.vertx.core.Promise;
import io.vertx.core.Vertx;
import io.vertx.core.VertxOptions;
import io.vertx.ext.web.Router;
import io.vertx.ext.web.handler.BodyHandler;

import java.util.concurrent.TimeUnit;

public class MainVerticle extends AbstractVerticle {

    public static void main(String[] args) {
        VertxOptions options = new VertxOptions()
            .setMaxWorkerExecuteTimeUnit(TimeUnit.SECONDS)
            .setMaxWorkerExecuteTime(20)
                .setMaxEventLoopExecuteTimeUnit(TimeUnit.SECONDS)
                .setMaxEventLoopExecuteTime(20);
        Vertx vertx = Vertx.vertx(options);
        vertx.deployVerticle(new MainVerticle());
    }

    @Override
    public void start(Promise<Void> startPromise) {
        Router router = Router.router(vertx);
        router.route().handler(BodyHandler.create());

        router.get("/bike").handler(ctx -> {
            OrderService.INSTANCE.findNearestVehicle(1, "scooter");
            ctx.response()
                    .putHeader("content-type", "application/json")
                    .end("{\"message\":\"Bike from Vert.x!\"}");
        });

        router.get("/scooter").handler(ctx -> {
            OrderService.INSTANCE.findNearestVehicle(2, "scooter");
            ctx.response()
                    .putHeader("content-type", "application/json")
                    .end("{\"message\":\"Scooter from Vert.x!\"}");
        });

        router.get("/car").handler(ctx -> {
            OrderService.INSTANCE.findNearestVehicle(3, "car");
            ctx.response()
                    .putHeader("content-type", "application/json")
                    .end("{\"message\":\"Car from Vert.x!\"}");
        });

        vertx.createHttpServer()
                .requestHandler(router)
                .listen(5000, http -> {
                    if (http.succeeded()) {
                        startPromise.complete();
                        System.out.println("HTTP server started on port 5000");
                    } else {
                        startPromise.fail(http.cause());
                    }
                });
    }
}

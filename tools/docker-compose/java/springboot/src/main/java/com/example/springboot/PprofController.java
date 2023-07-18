package com.example.springboot;

import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.ResponseBody;

import javax.servlet.http.HttpServletResponse;
import java.io.IOException;
import java.time.Duration;

import jpprof.CPUProfiler;

@RestController
public class PprofController {

  @GetMapping("/debug/pprof/profile")
  @ResponseBody
  public void profile(@RequestParam(required = false) String seconds, HttpServletResponse response) {
    try {
      Duration d = Duration.ofSeconds(Integer.parseInt(seconds));
      CPUProfiler.start(d, response.getOutputStream());
      response.flushBuffer();
    } catch (Exception e) {
      System.out.println("exception: " + e.getMessage());
    }
  }

}

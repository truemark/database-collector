package com.truemark.collector;

import com.amazonaws.services.lambda.runtime.Context;
import com.amazonaws.services.lambda.runtime.RequestHandler;

import java.util.Map;

/**
 * A simple AWS Lambda handler that returns a Hello World message.
 *
 * Handler: com.truemark.collector.HelloWorldHandler::handleRequest
 */
public class HelloWorldHandler implements RequestHandler<Map<String, Object>, String> {

    @Override
    public String handleRequest(Map<String, Object> input, Context context) {
        String requestId = context != null ? context.getAwsRequestId() : "unknown";
        String who = input != null && input.getOrDefault("name", "") instanceof String
                && !((String) input.get("name")).isBlank()
                ? (String) input.get("name")
                : "World";
        String message = "Hello, " + who + "!";
        if (context != null && context.getLogger() != null) {
            context.getLogger().log("RequestId=" + requestId + " input=" + String.valueOf(input));
            context.getLogger().log("Response=" + message);
        }
        return message;
    }
}

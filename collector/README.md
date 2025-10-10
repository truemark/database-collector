# Hello World Java Lambda (collector)

This is a minimal Java AWS Lambda handler that returns a greeting message.

- Handler class: `com.truemark.collector.HelloWorldHandler::handleRequest`
- Java version: 17
- Build tool: Maven

## Build

From this directory:

```bash
mvn -q -e -U -B clean package
```

The deployable artifact will be created at:

```
target/collector.jar
```

## Test locally (quick check)

You can quickly invoke the handler with the AWS Lambda Java runtime interface emulator or simply run a small Java snippet. Example using `jshell`:

```bash
jshell <<'EOF'
import com.truemark.collector.HelloWorldHandler;
import com.amazonaws.services.lambda.runtime.*;
import java.util.*;
System.out.println(new HelloWorldHandler().handleRequest(Map.of("name","Developer"), null));
EOF
```

Expected output:

```
Hello, Developer!
```

## Deploy

Package `target/collector.jar` with a Lambda runtime of Java 17 (e.g., `java17` in AWS Console/CDK). Set the handler to:

```
com.truemark.collector.HelloWorldHandler::handleRequest
```

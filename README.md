# KanoBUG Slack App server

Slack App server with slash command and interactive component hosting on AWS Lambda and DynamoDB.
You will need to put secured string (oauth token & verification token) via SSM, adjust *serverless.yml* as you please.


## Installation

You will need *Go*, *npm* and *serverless* framework installed. Please also create a Slack App and obtain the OAuth token with the correct scopes.

Once done, install npm packages with `npm install`, after that just simply run `serverless deploy` and it will build all binaries and push the Lambda to AWS.

Happy hacking!

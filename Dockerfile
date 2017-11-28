##
# Usage
#
# See the `docker-compose.yml` file and commands (recommended)
#  OR
# Build manually with `docker build . -t bitgo/prova`
#

# Use the latest 1.x golang alpine image
FROM golang:1-alpine

# Install Git (required for 'go get')
RUN apk add --update git

# Install Glide (Go dependency management)
RUN go get -u github.com/Masterminds/glide

# Create the working directory
WORKDIR $GOPATH/src/github.com/bitgo/prova

# Copy the source files into the container
COPY . .

# Install the project dependencies
RUN glide install

# Install Prova
RUN go install

# Run Prova (uses the testnet by default)
CMD prova --testnet
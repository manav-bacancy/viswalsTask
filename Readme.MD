# Viswals Backend Test
This project processes data from a CSV file by publishing it to a RabbitMQ queue. A consumer reads the data from the queue, processes it, and stores it in Redis (for caching) and PostgreSQL (for persistence). Additionally, a REST API is provided to retrieve and manage the stored data, enabling efficient and scalable data handling.

## Project Overview

### Producer
### Producer Tasks

1. Read data from a CSV file.
2. Parse the CSV data into a structured format (JSON).
3. Connect to RabbitMQ and declare a queue.
4. Publish the parsed data to the RabbitMQ queue.

### Consumer
### Consumer Tasks

1. Read data from the RabbitMQ queue.
2. Parse the data into a structured format.
3. Insert the processed data into Redis for caching.
4. Insert the processed data into PostgreSQL for persistent storage.
5. Act as an HTTP service, providing RESTful APIs to retrieve and manage the stored data.

## API Documentation

### APIs
| API Name         | HTTP Method | Path                | Description                                                         |
|------------------|-------------|---------------------|---------------------------------------------------------------------|
| Get All Users    | GET         | `/users`            | Fetch a list of all users                                           |
| Get User by ID   | GET         | `/users/{id}`       | Fetch a single user by their ID                                     |
| Get All Users SSE | GET         | `/users/sse`        | Fetch a list of all users and send to client using ServerSentEvents |
| Create User      | POST        | `/users`            | Create user to database                                             |
| Delete User     | DELETE      | `/users`            | Delete user from database                                           |


## How to Run

run `docker compose up --build` in root project directory (you can remove the `--build` flag after running one time)

and consumer will extend as API on `http://localhost:5000/`

## Test Case Coverage:

test case coverage:




This is a sample gameplay server that uses Nakama to proxy gameplay requests to a separate server.

To start nakama and the gameplay server:

```bash
./start.sh
```

After updating the gameplay server, the server can be rebuilt via:
```bash
./restart_server.sh
```

Note, if any server endpoints have been added or removed nakama must be relaunched (via the start.sh script)

Once nakama and the gameplay server are running, visit `localhost:7351` to access nakama. For local development, use `admin:password` as your login credentials.

The Account tab on the left will give you access to a valid account ID.

The API Explorer tab on the left will allow you to make requests to the gameplay server.

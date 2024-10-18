# AlgoRelay

AlgoRelay is nostr's first algorithmic relay. It allows any relay operator to build their own relay using a preset weighting algorithm. It is nostr native and is compatible with the current nostr protocol.

## How the Feed Algorithm Works

Our feed algorithm is designed to deliver a personalized and engaging experience by balancing posts from authors you interact with and viral content from across the network. It considers several key factors to surface posts that are both relevant and timely, while also highlighting popular content. Below is a breakdown of how each component contributes to the overall ranking of posts in your feed.

### Key Components of the Algorithm

1. **Interactions with Authors**

   - **Weight:** `WEIGHT_INTERACTIONS_WITH_AUTHOR`
   - Posts from authors you frequently engage with (through comments, reactions, or zaps) are given priority. The higher this weight, the more often you'll see posts from authors you regularly interact with.
   - **Why it matters:** This ensures that content from your favorite authors (people you've frequently interacted with) appears more prominently in your feed.

2. **Global Comments on Posts**

   - **Weight:** `WEIGHT_COMMENTS_GLOBAL`
   - The algorithm considers the total number of comments on each post across the platform. A higher weight here gives priority to posts with more comments, as they indicate meaningful engagement and discussions.
   - **Why it matters:** Posts with many comments often spark conversations and debates, making them potentially more interesting to include in your feed.

3. **Global Reactions on Posts**

   - **Weight:** `WEIGHT_REACTIONS_GLOBAL`
   - Reactions (such as likes or emojis) are another form of engagement. This weight determines how much reactions influence the ranking of a post.
   - **Why it matters:** Reactions are a quick way for users to show approval or interest, and posts with high reactions tend to resonate with the broader community.

4. **Global Zaps on Posts**

   - **Weight:** `WEIGHT_ZAPS_GLOBAL`
   - Zaps represent a more significant form of interaction, as they involve a financial transaction (usually a small amount of Bitcoin). The algorithm boosts posts with a higher number of zaps, as they indicate strong support.
   - **Why it matters:** Zaps signal high value and endorsement from other users, making these posts stand out in your feed.

5. **Recency**

   - **Weight:** `WEIGHT_RECENCY`
   - Newer posts are generally more relevant, and this weight controls how much the algorithm favors recent content.
   - **Why it matters:** Fresh content is given a boost to ensure that your feed stays up-to-date with the latest posts. The recency factor ensures that older posts gradually decay in importance over time.

6. **Viral Posts**

   - **Threshold:** `VIRAL_THRESHOLD`
   - Posts that exceed a certain number of combined comments, reactions, and zaps are considered viral. Viral posts are ranked higher in the feed based on their total engagement, but a dampening factor is applied to ensure they don't overwhelm your feed.
   - **Dampening Factor:** `VIRAL_POST_DAMPENING`
   - Viral posts are exciting, but they shouldn't dominate your feed. This dampening factor reduces the influence of viral posts, ensuring a balance between personal relevance and global popularity.
   - **Why it matters:** Viral posts add variety and surface popular content, but they are balanced with content from authors you personally interact with to maintain a well-rounded feed.

7. **Decay Rate for Recency**
   - **Rate:** `DECAY_RATE`
   - This controls how quickly older posts lose relevance. A higher decay rate means that older posts will decay in importance faster, while a lower decay rate keeps older posts in the feed for longer.
   - **Why it matters:** This ensures that the feed doesn't become too stale by over-prioritizing older posts. It keeps the feed dynamic and responsive to new content.

### How it All Comes Together

The feed combines two main components: posts from authors you frequently interact with and viral posts from across the network. Each post is scored based on the factors outlined above, with more weight given to interactions with familiar authors, balanced by global engagement metrics (comments, reactions, zaps), and adjusted for recency. The result is a feed that feels personalized while keeping you informed of the most popular content on the platform.

Viral posts are dampened by the `VIRAL_POST_DAMPENING` factor to ensure they don’t overshadow posts from authors you frequently interact with. Additionally, posts from the user’s own account are filtered out to avoid cluttering the feed with self-posts.

With this algorithm, users get a curated mix of familiar and trending content, ensuring that their feed is always engaging and relevant.

## Prerequisites

- **Go**: Ensure you have Go installed on your system. You can download it from [here](https://golang.org/dl/).
- **Build Essentials**: If you're using Linux, you may need to install build essentials. You can do this by running `sudo apt install build-essential`.
- **PostgreSQL**: You'll need a PostgreSQL database to store the relay data. You can use the included docker-compose file to set up a PostgreSQL database if you don't have one already.

## Setup Instructions

Follow these steps to get the Algo Relay running on your local machine:

### 1. Clone the repository

```bash
git clone https://github.com/bitvora/algo-relay.git
cd algo-relay
```

### 2. Copy `.env.example` to `.env`

You'll need to create an `.env` file based on the example provided in the repository.

```bash
cp .env.example .env
```

### 3. Set your environment variables

Open the `.env` file and set the necessary environment variables.

### 4. Build the project

Run the following command to build the relay:

```bash
go build
```

If you do not have a postgres database set up, you can use the included docker-compose file to set up a PostgreSQL database:

```bash
docker-compose up -d
```

### 5. Create a Systemd Service

To have the relay run as a service, create a systemd unit file. Make sure to limit the memory usage to less than your system's total memory to prevent the relay from crashing the system.

1. Create the file:

```bash
sudo nano /etc/systemd/system/algo.service
```

2. Add the following contents:

```ini
[Unit]
Description=Algo Relay
After=network.target

[Service]
ExecStart=/home/ubuntu/algo-relay/algo-relay
WorkingDirectory=/home/ubuntu/algo-relay
Restart=always

[Install]
WantedBy=multi-user.target
```

3. Reload systemd to recognize the new service:

```bash
sudo systemctl daemon-reload
```

4. Start the service:

```bash
sudo systemctl start algo
```

5. (Optional) Enable the service to start on boot:

```bash
sudo systemctl enable algo
```

### 6. Serving over nginx (optional)

You can serve the relay over nginx by adding the following configuration to your nginx configuration file:

```nginx
server {
    listen 80;
    server_name yourdomain.com;

    location / {
        proxy_pass http://localhost:3334;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

Replace `yourdomain.com` with your actual domain name.

After adding the configuration, restart nginx:

```bash
sudo systemctl restart nginx
```

### 7. Install Certbot (optional)

If you want to serve the relay over HTTPS, you can use Certbot to generate an SSL certificate.

```bash
sudo apt-get update
sudo apt-get install certbot python3-certbot-nginx
```

After installing Certbot, run the following command to generate an SSL certificate:

```bash
sudo certbot --nginx
```

Follow the instructions to generate the certificate.

### 8. Run The Import (optional)

If you want to import your old notes and notes you're tagged in from other relays, run the following command:

```bash
./algo-relay --import
```

### 9. Access the relay

Once everything is set up, the relay will be running on `localhost:3334` with the following endpoints:

- `localhost:3334`

## License

This project is licensed under the MIT License.

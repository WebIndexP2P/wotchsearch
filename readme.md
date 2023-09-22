# WotchSearch

WotchSearch is a text indexing backend for Wotch to give users the ability to search the entire video collection on a given WebIndexP2P tree.

There are 3 primary components to the program:

1. A permanent connection to a WebIndexP2P node that will synchronize with the existing data and keep track of new data. It checks for the presence of "wotch" namespace in the root document, and adds any playlist CIDs to a scan queue.

2. The playlist IPFS scanner, this will continually check all existing wotch accounts that have missing IPFS CIDs and connect to suggested IPFS gateways and attempt to download the content. Any videos found will be added to the text index.

3. The text index API, external Wotch UIs will use this API to quickly search all videos by name, video title, and allows searching for a video source link perhaps to find an alternative source for the same video.

## Operational modes

This program can be configured several ways:
* Index all videos in the tree
* Index only videos subscribed by specific account
* Pin/Seed indexed content to a specific IPFS node
  * Playlist data
  * Thumbnails
  * Ipfs videos

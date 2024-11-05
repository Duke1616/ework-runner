from wechatpy.enterprise import WeChatClient


class Wechat:
    def __init__(self, wechatClient: WeChatClient):
        self.client = wechatClient

    def get_user_info(self, user_id: str):
        return self.client.user.get(user_id)

    def add_media(self, media_type, media_file):
        return self.client.media.upload(media_type, media_file)

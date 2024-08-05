from wechatpy.enterprise import WeChatClient

class Wechat:
    def __init__(self, wechatClient: WeChatClient):
        self.client = wechatClient

    def get_user_info(self, user_id: str):
        return self.client.user.get(user_id)




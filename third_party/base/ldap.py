from ldap3 import Server, Connection, ALL, SUBTREE, DEREF_ALWAYS, ObjectDef, AttrDef
import logging
logger = logging.getLogger(__name__)


class Ldap:
    def __init__(self, base_dn: str, url: str):
        self.conn = Connection
        self.base_dn = base_dn
        self.url = url
        self.search_user_filter = '(&(objectclass=person)(sAMAccountName={}))'
        self.ret_attrs = ['SAMAccountName', 'memberOf']
        self.size_limit = 0
        self.time_limit = 0

    def bind(self, user: str, passwd: str):
        server = Server(self.url, connect_timeout=5, use_ssl=False, get_info=ALL)
        conn = Connection(server, user=user, password=passwd)
        if not conn.bind():
            print('登录绑定失败：', conn.result)
            return
        self.conn = conn

    def unbind(self):
        self.conn.unbind()

    def search_user(self, account_name: str):
        formatted_filter = self.search_user_filter.format(account_name)
        self.conn.search(search_base=self.base_dn, search_filter=formatted_filter,
                         search_scope=SUBTREE, attributes=self.ret_attrs,
                         size_limit=self.size_limit, time_limit=self.time_limit,
                         types_only=False, dereference_aliases=DEREF_ALWAYS)

        # 检查获取的值是否为空或者长度是否不为1
        entries_value = getattr(self.conn, 'entries')
        if not entries_value and len(entries_value) != 1:
            return

        return self.conn.entries

    def create_user(self, account_name: str, username: str, ou: str, default_pwd: str):
        # 如果账号存在，则直接返回
        if self.search_user(account_name):
            return

        # 如果名字4字以上，一律按照阜新复姓 上官、独孤
        if len(username) > 3:
            compound_family_name = username[:2]
            given_name = username[2:]
        else:
            compound_family_name = username[0]
            given_name = username[1:]

        # 拼接邮箱
        lower_base_dn = self.base_dn.lower()
        mail_domain = lower_base_dn.replace('dc=', '').replace(',', '.')
        mail = account_name + '@' + mail_domain

        # 属性信息
        user_attributes = {
            'sn': compound_family_name,
            'givenName': given_name,
            'name': account_name,
            'mail': mail,
            'userPrincipalName': mail,
            'SAMAccountName': account_name,
            'displayName': username,
            'objectClass': ['top', 'person', 'organizationalPerson', 'user']
        }

        # 添加用户
        user_dn = 'cn={},ou={},{}'.format(username, ou, self.base_dn)
        print(user_dn)
        result = self.conn.add(user_dn, attributes=user_attributes)

        # 检查添加结果
        if result:
            self.modify_password(user_dn, default_pwd)
        else:
            error_message = self.conn.result['message']
            print(f"用户添加失败，, 错误消息：{error_message}")

        # 关闭连接
        self.unbind()

    def add_members_to_groups(self, user_dn, group_dn):
        # 不处理错误情况
        return self.conn.extend.microsoft.add_members_to_groups(user_dn, group_dn)

    def modify_password(self, user_dn: str, default_pwd: str):
        self.conn.extend.microsoft.modify_password(user_dn, default_pwd)
        user_attributes = {
            # 用户状态为启用
            'userAccountControl': [('MODIFY_REPLACE', 512)],
            # 用户下次登录需要重置密码
            'pwdLastSet': [('MODIFY_REPLACE', 0)]
        }

        result = self.conn.modify(user_dn, user_attributes)
        if result:
            print("修改用户成功")
        else:
            error_message = self.conn.result['message']
            print(f"用户添加密码，, 错误消息：{error_message}")
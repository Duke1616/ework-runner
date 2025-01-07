from ldap3 import Server, Connection, ALL, SUBTREE, DEREF_ALWAYS, MODIFY_ADD, MODIFY_REPLACE
import logging

logger = logging.getLogger(__name__)


class Ldap:
    def __init__(self, base_dn: str, url: str):
        self.conn = None
        self.base_dn = base_dn
        self.url = url
        self.search_user_filter = '(&(objectclass=person)(cn={}))'
        self.ret_attrs = ['cn']
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

    def find_or_create_user(self, account_name: str, username: str, ou: str, title: str, default_pwd: str):
        try:
            entity = self.search_user(account_name)
            if entity:
                return entity

            return self.create_user(account_name, username, ou, title, default_pwd)
        except Exception as e:
            error_message = f"用户名 => [{account_name}]，错误信息 => {str(e)}"
            return Exception(error_message)

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

    def create_user(self, account_name: str, username: str, ou: str, title: str, default_pwd: str):
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
            'mail': mail,
            'title': title,
            'displayName': username,
            'userPassword': default_pwd,
            'objectClass': ['top', 'person', 'organizationalPerson', 'inetOrgPerson']
        }

        # 添加用户
        user_dn = 'cn={},ou={},{}'.format(account_name, ou, self.base_dn)
        result = self.conn.add(dn=user_dn, attributes=user_attributes)

        # 检查添加结果
        if not result:
            raise Exception(self.conn.result['message'])

    def add_members_to_groups(self, user_dn, group_dn):
        # 检查用户是否已经是组的成员
        self.conn.search(group_dn, '(objectClass=*)', attributes=['member'])
        if self.conn.entries:
            group_entry = self.conn.entries[0]
            current_members = group_entry.member.values if 'member' in group_entry else []
            if user_dn in current_members:
                print(f"用户 {user_dn} 已经存在组 {group_dn}. 跳过.")
                return True

        # 如果用户不是成员，则添加到组
        changes = {
            'member': [(MODIFY_ADD, [user_dn])]
        }
        result = self.conn.modify(group_dn, changes)
        if not result:
            raise Exception(f"用户添加组失败: {self.conn.result['message']}")
        print(f"用户 {user_dn} 成功添加到组 {group_dn}.")
        return result

    def modify_password(self, user: str, new_pwd: str):
        user_entry = self.search_user(user)
        if not user_entry:
            print(f"用户 {user} 不存在")
            return False

        # 获取用户的 DN
        user_dn = user_entry[0].entry_dn

        changes = {
            'userPassword': [(MODIFY_REPLACE, [new_pwd])]
        }
        return self.conn.modify(user_dn, changes)


    def verify_user_credentials(self, user: str, passwd: str):
        """
        验证用户的凭据（用户名和密码）。

        :param user: 用户
        :param passwd: 密码
        :return: 如果凭据有效返回 True，否则返回 False
        """
        try:
            # 搜索用户
            user_entry = self.search_user(user)
            if not user_entry:
                print(f"用户 {user} 不存在")
                return False

            # 获取用户的 DN
            user_dn = user_entry[0].entry_dn

            # 尝试使用提供的用户名和密码绑定到 AD
            server = Server(self.url, connect_timeout=5, use_ssl=False, get_info=ALL)
            conn = Connection(server, user=user_dn, password=passwd)

            if conn.bind():
                print(f"用户 {user} 的凭据验证成功")
                conn.unbind()
                return True
            else:
                print(f"用户 {user} 的凭据验证失败：{conn.result['message']}")
                return False

        except Exception as e:
            print(f"验证用户 {user} 的凭据时发生错误：{str(e)}")
            return False
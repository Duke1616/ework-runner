from ldap3 import Server, core, Connection, ALL, NTLM, SUBTREE, DEREF_ALWAYS
import logging
logger = logging.getLogger(__name__)


class Ldap:
    def __init__(self, variables):
        self.conn = ''
        self.search_base = 'dc=ebondhm,dc=com'
        self.search_filter = '(&(sAMAccountName=*)(memberOf=cn=gitlab,dc=ebondhm,dc=com))'
        self.search_scope = SUBTREE
        self.ret_attrs = ['SAMAccountName', 'cn', 'mail', 'sn', 'title']
        self.size_limit = 0
        self.time_limit = 0
        self.types_only = False
        self.deref_aliases = DEREF_ALWAYS

    def bind(self, user: str, passwd: str):
        server = Server('10.31.0.50', port=389, use_ssl=False, get_info=ALL)
        conn = Connection(server, user=user, password=passwd)
        if not conn.bind():
            print('登录绑定失败：', conn.result)
        self.conn = conn

    def search(self):
        self.conn.search(search_base=self.search_base, search_filter=self.search_filter, search_scope=self.search_scope,
                         attributes=self.ret_attrs, size_limit=self.size_limit, time_limit=self.time_limit,
                         types_only=self.types_only, dereference_aliases=self.deref_aliases)

        if not self.conn.entries:
            return

        for entry in self.conn.response:
            print('CN:', entry)
            pass

    def create_user(self):
        pass

    def modify(self):
        pass


ldap = Ldap("1")
ldap.bind("CN=bot,OU=bots,DC=ebondhm,DC=com", "20EQ2B$d!Fy2C8TNKF1f")
ldap.search()

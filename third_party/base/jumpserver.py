from httpsig.requests_auth import HTTPSignatureAuth
import requests, datetime, json


signature_headers = ['(request-target)', 'accept', 'date']
gmt_form = '%a, %d %b %Y %H:%M:%S GMT'


class JumpServer:
    def __init__(self,  jms_url: str, username: str, displayName: str):
        self.auth = HTTPSignatureAuth
        self.jms_url = jms_url
        self.username = username
        self.displayName = displayName
        self.headers = {
            'Accept': 'application/json',
            'X-JMS-ORG': '00000000-0000-0000-0000-000000000002',
            'Date': datetime.datetime.utcnow().strftime(gmt_form)
        }

    def get_auth(self, KeyID, SecretID):
        self.auth = HTTPSignatureAuth(key_id=KeyID, secret=SecretID, algorithm='hmac-sha256', headers=signature_headers)

    def create_user(self, email: str):
        url = self.jms_url + '/api/v1/users/users/'
        data = {
            "source": "ldap",
            "username": self.username,
            "name": self.displayName,
            "email": email,
            "mfa_level": 0,
            "is_active": True
        }
        try:
            response = requests.post(url, auth=self.auth, headers=self.headers, json=data)
            response.raise_for_status()  # 检查请求是否成功
            return response.json()
        except requests.RequestException as e:
            return Exception(f"创建用户: {self.username}. 错误: {str(e)}")

    def get_user(self):
        url = self.jms_url + '/api/v1/users/users/'
        params = {
            "username": self.username
        }
        try:
            response = requests.get(url, auth=self.auth, headers=self.headers, params=params)
            response.raise_for_status()
            return response.json()
        except requests.RequestException as e:
            return Exception(f"查询用户: {self.username}. 错误: {str(e)}")

    def create_system_user(self, cmd_filters: list):
        url = self.jms_url + '/api/v1/assets/system-users/'
        data = {
            "name": self.displayName,
            "username": self.username,
            "sudo": "/bin/whoami,/usr/bin/docker",
            "sftp_root": "/home/" + self.username,
            "home": "/home/" + self.username,
            "protocol": "ssh",
            "login_mode": "auto",
            "auto_push": True,
            "cmd_filters": cmd_filters
        }
        try:
            response = requests.post(url, auth=self.auth, headers=self.headers, json=data)
            response.raise_for_status()
            return response.json()
        except requests.RequestException as e:
            return Exception(f"创建系统用户: {self.username}. 错误: {str(e)}")

    def get_system_user(self):
        url = self.jms_url + '/api/v1/assets/system-users/'
        params = {
            "username": self.username
        }
        try:
            response = requests.get(url, auth=self.auth, headers=self.headers, params=params)
            response.raise_for_status()
            return response.json()
        except requests.RequestException as e:
            return Exception(f"查询系统用户: {self.username}. 错误: {str(e)}")

    def get_assets_by_host_ip(self, ip: str):
        url = self.jms_url + '/api/v1/assets/assets/'
        params = {
            "ip": ip
        }
        try:
            response = requests.get(url, auth=self.auth, headers=self.headers, params=params)
            response.raise_for_status()
            return response.json()
        except requests.RequestException as e:
            return Exception(f"查询主机 IP 错误: {ip}. 错误: {str(e)}")

    def create_asset_permissions(self, userId: str, sysUserId: str, assetIds: list):
        url = self.jms_url + '/api/v1/perms/asset-permissions/'
        data = {
            "name": self.displayName,
            "users": userId,
            "system_users": sysUserId,
            "is_active": True,
            "assets": assetIds
        }
        try:
            response = requests.post(url, auth=self.auth, headers=self.headers, json=data)
            response.raise_for_status()
            return response.json()
        except requests.RequestException as e:
            return Exception(f"创建资产授权错误: {userId}. 错误: {str(e)}")

    def add_asset_permissions(self, asset: str, assetPermission: str):
        url = self.jms_url + '/api/v1/perms/asset-permissions-assets-relations/'
        data = {
            "asset": asset,
            "assetpermission": assetPermission,
        }
        try:
            response = requests.post(url, auth=self.auth, headers=self.headers, json=data)
            response.raise_for_status()
            return response.json()
        except requests.RequestException as e:
            return Exception(f"添加资产授权错误: 资产: {asset} 授权: {assetPermission}. 错误: {str(e)}")

    def get_asset_permissions(self, userId: str, sysUserId: str):
        url = self.jms_url + '/api/v1/perms/asset-permissions/'
        params = {
            "user_id": userId,
            "system_user_id": sysUserId
        }
        try:
            response = requests.get(url, auth=self.auth, headers=self.headers, params=params)
            response.raise_for_status()
            return response.json()
        except requests.RequestException as e:
            return Exception(f"查询资产授权错误: {userId}. 错误: {str(e)}")

    def find_or_create_system_user(self, cmd_filters: list):
        sysUser = self.get_system_user()
        if len(sysUser) != 1:
            raise Exception(f"系统用户不唯一: {self.displayName}. 返回: {sysUser}")
        if sysUser:
            print(f"系统用户已经存在: {self.username}")
            return sysUser[0]["id"]

        return self.create_system_user(cmd_filters)

    def find_or_create_user(self, email: str):
        user = self.get_user()
        if len(user) != 1:
            raise Exception(f"用户不唯一: {self.displayName}. 返回: {user}")
        if user:
            print(f"用户已经存在: {self.username}")
            return user[0]["id"]

        return self.create_user(email)

    def find_or_create_or_add_permissions(self, userId: str, sysUserId: str, ips: list):
        asset_perms = self.get_asset_permissions(userId, sysUserId)
        if len(asset_perms) != 1:
            raise Exception(f"资产授权查询不唯一: {userId}. 返回: {asset_perms}")

        assetIds = []
        for ip in ips:
            ip = self.get_assets_by_host_ip(ip)
            assetIds.append(ip[0]["id"])

        # 如果资产授权存在，添加资产
        if asset_perms:
            for assetId in assetIds:
                self.add_asset_permissions(assetId, asset_perms[0]["id"])
            return self.add_asset_permissions

        # 如果资产授权不存在，创建资产授权
        return self.create_asset_permissions(userId, sysUserId, assetIds)

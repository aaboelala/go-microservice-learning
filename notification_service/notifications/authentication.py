import logging
import jwt
from django.conf import settings
from rest_framework import authentication, exceptions

logger = logging.getLogger(__name__)


class AuthenticatedUser:
    """
    Represents an authenticated user principal derived exclusively from verified JWT claims.

    Engineering Decisions & Educational Notes:
    -------------------------------------------
    1. Stateless Authentication Principal:
       In a distributed microservice architecture, the Notification Service does not maintain
       a local table of users (which belongs to the User Service). Instead, this lightweight
       principal represents the authenticated subject during request processing.
       Django's `request.user` receives this object with `.id`, `.pk`, and `.is_authenticated`.
    """
    def __init__(self, user_id: int, role: str = "user"):
        self.id = user_id
        self.pk = user_id
        self.role = role
        self.is_authenticated = True

    def __str__(self):
        return f"AuthenticatedUser(id={self.id}, role={self.role})"

    @property
    def is_anonymous(self):
        return False


class JWTAuthentication(authentication.BaseAuthentication):
    """
    Custom JWT Authentication class validating tokens issued by the Go ecosystem.

    Engineering Decisions & Educational Notes:
    -------------------------------------------
    1. Why Notification Ownership Comes from JWT Authentication:
       Ownership of notification resources is cryptographically proven via the HMAC-SHA256
       signature on the token. The user ID claim is extracted directly from the verified
       payload and used by views to scope all database queries. The service NEVER trusts
       a `user_id` parameter sent in HTTP headers, URL params, or JSON bodies.

    2. Contract Compatibility with Go Order Service:
       The Go Order Service and User Service sign tokens using HMAC SHA-256 with claims:
         - `user_id` (uint): Primary user identifier.
         - `id` (uint, optional): Fallback identifier.
         - `sub` (string, optional): Registered subject claim fallback.
         - `role` (string): User role (e.g. "user", "admin").
         - `exp` (int): Standard token expiration timestamp.
    """

    def authenticate(self, request):
        auth_header = request.headers.get('Authorization')
        if not auth_header:
            return None

        parts = auth_header.split()
        if len(parts) != 2 or parts[0].lower() != 'bearer':
            raise exceptions.AuthenticationFailed(
                'Invalid Authorization header format. Expected "Bearer <token>"'
            )

        token = parts[1].strip()
        if not token:
            raise exceptions.AuthenticationFailed('Token cannot be empty')

        secret_key = settings.JWT_CONFIG.get('SECRET_KEY')
        algorithm = settings.JWT_CONFIG.get('ALGORITHM', 'HS256')

        if not secret_key:
            logger.error("JWT_SECRET_KEY is not configured in settings.")
            raise exceptions.AuthenticationFailed('Server authentication misconfigured')

        try:
            payload = jwt.decode(
                token,
                secret_key,
                algorithms=[algorithm],
                options={"require": ["exp"]}
            )
        except jwt.ExpiredSignatureError:
            raise exceptions.AuthenticationFailed('Token has expired')
        except jwt.InvalidTokenError as e:
            raise exceptions.AuthenticationFailed(f'Invalid token: {str(e)}')

        # Extract user_id following the exact contract of Go TokenHelper
        user_id = payload.get('user_id') or payload.get('id')
        if not user_id and 'sub' in payload:
            try:
                user_id = int(payload['sub'])
            except (ValueError, TypeError):
                pass

        if not user_id or user_id <= 0:
            raise exceptions.AuthenticationFailed('User ID is missing or invalid in token claims')

        user = AuthenticatedUser(
            user_id=int(user_id),
            role=payload.get('role', 'user')
        )
        return (user, token)

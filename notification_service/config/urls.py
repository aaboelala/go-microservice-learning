from django.contrib import admin
from django.urls import path, include
from rest_framework.schemas import get_schema_view
from rest_framework.permissions import AllowAny
from notifications.views import liveness_check, readiness_check

urlpatterns = [
    path('admin/', admin.site.urls),
    path('api/v1/', include('notifications.urls')),
    path('api/schema/', get_schema_view(
        title="Notification Service API",
        description="RESTful In-App Notification Management System",
        version="1.0.0",
        permission_classes=[AllowAny],
    ), name='openapi-schema'),
    path('health/liveness/', liveness_check, name='root_liveness'),
    path('health/readiness/', readiness_check, name='root_readiness'),
]

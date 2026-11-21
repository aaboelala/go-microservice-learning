from django.core.management.base import BaseCommand
from notifications.consumers.order_consumer import OrderEventConsumer

class Command(BaseCommand):
    help = 'Starts the RabbitMQ Consumer to listen for Order events'

    def handle(self, *args, **options):
        self.stdout.write(self.style.SUCCESS('Starting Order Events RabbitMQ Consumer...'))
        consumer = OrderEventConsumer()
        try:
            consumer.start_consuming()
        except KeyboardInterrupt:
            self.stdout.write(self.style.WARNING('Consumer stopped by user.'))
